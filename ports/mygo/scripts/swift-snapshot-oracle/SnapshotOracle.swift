// SPDX-License-Identifier: AGPL-3.0-only
// Isolated synthetic capture only. Never calls Store, package.write/read, SQLite or App.
import Foundation
import Darwin
import NoonmarkCore
import NoonmarkStorage

func instrumentJSON(_ value:J) throws -> Data {
    _ = try conservativeBudget(value)
    return try JSONSerialization.data(withJSONObject:value.foundation(),options:[.sortedKeys,.withoutEscapingSlashes])
}
func optionalText(_ value:J, key:String="utf8Hex") throws -> String? {
    try demand(Set(value.object.keys)==["present",key])
    let hex=value[key].string
    try demand(hex.utf8.count<=8192 && hex.count%2==0,"resource-limit")
    try demand(hex.allSatisfy {"0123456789abcdef".contains($0)})
    if value["present"] == .bool(false) {try demand(hex.isEmpty);return nil}
    try demand(value["present"] == .bool(true))
    var bytes:[UInt8]=[];var p=hex.startIndex
    while p<hex.endIndex {let q=hex.index(p,offsetBy:2);bytes.append(UInt8(hex[p..<q],radix:16)!);p=q}
    guard let text=String(bytes:bytes,encoding:.utf8) else {throw OracleError.rejected("schema-invalid")};return text
}
func date(_ value:J)throws->LocalDate {
    try validateType(value,"LocalDate")
    func n(_ k:String)->Int {if case .number(let x)=value[k] {return Int(x)!};return 0}
    return LocalDate(year:n("year"),month:n("month"),day:n("day"))
}
func profile(_ value:J, export:Bool=false) throws {
    try validateType(value,"Envelope");try demand(value["formatVersion"] == .number("7"))
    var parser=try Scanner(Data(preferencesJSON.utf8));var preferences=try parser.parse().object
    if export {var sync=preferences["localFirstSyncPolicy"]!.object;sync["enabled"] = .bool(false);preferences["localFirstSyncPolicy"] = .object(sync)}
    try demand(value["snapshot"]["preferences"] == .object(preferences),"profile-violation")
    let snapshot=value["snapshot"]
    var entities=["days","chains","definitions","traces","subtasks"].reduce(0){$0+snapshot[$1].array.count}
    for row in snapshot["definitions"].array {entities += row["plannedSubtasks"].array.count}
    for name in ["chains","traces"] {for row in snapshot[name].array {entities += row["noteEntries"].array.count}}
    try demand(entities<=96,"resource-limit")
}
// These files are transport artifacts, not completed case observations. The
// controller must re-archive them into its run-wide ledger before acceptance.
final class Archive {
    let root:URL
    var bytes=0,count=0
    var artifacts:[String:[J]]=[:]
    init(_ path:String)throws {
        root=URL(fileURLWithPath:path);var info=stat()
        try demand(lstat(path,&info) != 0,"source-mismatch")
        try FileManager.default.createDirectory(at:root,withIntermediateDirectories:false)
        try demand(lstat(path,&info)==0 && (info.st_mode&mode_t(S_IFMT))==mode_t(S_IFDIR),"source-mismatch")
        try demand(try FileManager.default.contentsOfDirectory(atPath:path).isEmpty,"source-mismatch")
    }
    func save(_ point:String,_ layer:String,_ raw:Data)throws {
        try demand(["seed","create-one","schedule-one","cross"].contains(point))
        try demand(["swift-engine-projection","swift-package","swift-export","swift-self-reencoded","swift-cross-projection"].contains(layer))
        try demand(raw.count<=262144 && count<18 && bytes+raw.count<=4194304,"resource-limit")
        let folder=root.appendingPathComponent(point,isDirectory:true)
        if !FileManager.default.fileExists(atPath:folder.path) {try FileManager.default.createDirectory(at:folder,withIntermediateDirectories:false)}
        let path=folder.appendingPathComponent(layer+".json").path
        let fd=open(path,O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW,0o600)
        try demand(fd>=0,"missing-evidence");defer{close(fd)}
        try raw.withUnsafeBytes { buffer in
            var offset=0
            while offset<raw.count {let n=write(fd,buffer.baseAddress!.advanced(by:offset),raw.count-offset);try demand(n>0,"missing-evidence");offset += n}
        }
        count += 1;bytes += raw.count
        artifacts[point,default:[]].append(.object(["name":.string(point+"/"+layer+".json"),"bytes":.number(String(raw.count)),"sha256":.string(digest(raw)),"rawState":.string("archived-unchecked")]))
    }
    func decoded(_ value:J)throws {
        let raw=try instrumentJSON(value);let url=root.appendingPathComponent("decoded.json")
        try raw.write(to:url,options:.withoutOverwriting)
    }
    func result(_ value:J)throws {
        let bytes=try JSONSerialization.data(withJSONObject:value.foundation(),options:[.sortedKeys,.withoutEscapingSlashes])
        try demand(bytes.count<=131072,"resource-limit")
        let path=root.appendingPathComponent("result.json").path
        let fd=open(path,O_WRONLY|O_CREAT|O_EXCL|O_NOFOLLOW,0o600);try demand(fd>=0,"missing-evidence");defer{close(fd)}
        try bytes.withUnsafeBytes {p in var o=0;while o<bytes.count {let n=write(fd,p.baseAddress!.advanced(by:o),bytes.count-o);try demand(n>0,"missing-evidence");o += n}}
    }
}
func predictCommand(_ before:J,_ command:J,_ note:String?)throws {
    try demand((note?.utf8.count ?? 0)<=4096 && command["titleUTF8Hex"].string.count<=8192 && command["description"]["utf8Hex"].string.count<=8192,"resource-limit")
    let s=before["snapshot"];let op=command["op"].string
    var addedText=0
    if op=="create" {
        try demand(s["chains"].array.count<16 && s["definitions"].array.count<16,"resource-limit")
        addedText=command["titleUTF8Hex"].string.count/2+command["description"]["utf8Hex"].string.count/2+(note?.utf8.count ?? 0)
    } else {
        let owner=command["chainRawID"].string
        let definitions=s["definitions"].array.filter{$0["chainID"]["rawValue"].string==owner}
        try demand(definitions.count==1,"mapping-ambiguous")
        let planned=definitions[0]["plannedSubtasks"].array
        try demand(planned.count<=2 && s["traces"].array.count<16 && s["subtasks"].array.count+planned.count<=16 && s["days"].array.count<16,"resource-limit")
        addedText=planned.reduce(0){$0+$1["title"].string.utf8.count}+definitions[0]["descriptionText"].string.utf8.count
        for chain in s["chains"].array where chain["id"]["rawValue"].string==owner {addedText += chain["noteEntries"].array.reduce(0){$0+$1["body"].string.utf8.count}}
    }
    // 400 nodes covers every fixed create/schedule field, IDs, one day and two
    // children. It deliberately overestimates, never allocates from input sizes.
    let bound=try conservativeBudget(before)+400*128+addedText*6
    try demand(bound<=262144 && bound*9<=4194304,"resource-limit")
}
@main struct SnapshotOracle {
    static func main() {
        do {try run()} catch {
            // No unbounded SDK error text or synthetic success on rejection.
            let reason: String
            if case OracleError.rejected(let r)=error {reason=r} else {reason="domain-or-codec-rejected"}
            FileHandle.standardError.write(Data((reason+"\n").utf8));exit(1)
        }
    }
    static func run(_ supplied:[String:String]? = nil) throws {
        let args=supplied == nil ? Array(CommandLine.arguments.dropFirst()) : [];try demand(args.count%2==0)
        var options:[String:String]=supplied ?? [:]
        for i in stride(from:0,to:args.count,by:2) {try demand(options[args[i]]==nil);options[args[i]]=args[i+1]}
        if let mode=options["--ingress-matrix"]{try demand(Set(options.keys)==["--ingress-matrix","--spec","--output"]);try runIngressMatrix(mode,spec:options["--spec"]!,output:options["--output"]!);return}
        if let mutation=options["--mutation-id"] {
            try demand(Set(options.keys)==["--mutation-id","--spec","--output"])
            try runMutation(mutation,spec:options["--spec"]!,output:options["--output"]!)
            return
        }
        if options["--cross"] != nil {
            try demand(Set(options.keys)==["--cross","--spec","--output"])
            let archive=try Archive(options["--output"]!)
            var decodeCalls=0
            do {
                let bytes=try readLimited(options["--cross"]!,262144)
                let parsed=try parseSnapshot(bytes,entry:"cross");try profile(parsed)
                // Original bytes; no canonicalization, ID substitution or sorting.
                decodeCalls += 1
                let decoded=try oracleDecode(bytes)
                let projection=try oracleProjection(decoded);try profile(projection)
                try archive.decoded(projection)
                try archive.result(.object(["schemaVersion":.number("2"),"accepted":.bool(true),"requiresClassificationAudit":.bool(false),"decodeCalls":.number(String(decodeCalls))]))
            } catch {
                try archive.result(.object(["schemaVersion":.number("2"),"accepted":.bool(false),"requiresClassificationAudit":.bool(false),"decodeCalls":.number(String(decodeCalls))]))
            }
            return
        }
        try demand(Set(options.keys)==["--fixture","--case","--vectors","--spec","--output"])
        let archive=try Archive(options["--output"]!)
        var points:[J]=[]
        var caseID:J = .string("")
        var phase="preflight"
        do {
        let spec=options["--spec"]!
        let fieldBytes=try readLimited(URL(fileURLWithPath:spec).appendingPathComponent("FieldSchema-R2.json").path,65536)
        try demand(digest(fieldBytes)=="7a0940e9a1fbc7b847aaa7ea059042e008faaac74b132b8300d0fbb35341d07c","source-mismatch")
        _ = try scannerIngress("config-FieldSchema-R2.json",fieldBytes)
        let registryBytes=try readLimited(URL(fileURLWithPath:spec).appendingPathComponent("SeedRegistry-R2.json").path,65536)
        try demand(digest(registryBytes)=="b15a50ddaa5315913aceb3d2b14bb823caf57580c7dd8e9c2db32bb71914032a","source-mismatch")
        let registry=try scannerIngress("config-SeedRegistry-R2.json",registryBytes)
        let caseValue=try scannerIngress("config-case",readLimited(options["--case"]!,16384));caseID=caseValue["id"];try demand(registry["cases"].array.contains(caseValue),"registry-mismatch")
        let seeds=registry["seeds"].array.filter{$0["id"]==caseValue["seed"]};try demand(seeds.count==1,"registry-mismatch")
        let fixture=try readLimited(options["--fixture"]!,262144);try demand(digest(fixture)==seeds[0]["sha256"].string,"registry-mismatch")
        let vectorBytes=try readLimited(options["--vectors"]!,16384);try demand(digest(vectorBytes)==vectorSHA256,"registry-mismatch")
        let vectors=try scannerIngress("config-NoteVectors-R2.json",vectorBytes)
        let fixtureView=try parseSnapshot(fixture,entry:"seed-"+caseValue["seed"].string);try profile(fixtureView);_ = try conservativeBudget(fixtureView)
        // Fixture-only canonical preparation. Never used for cross-decode.
        let prepared=try JSONSerialization.data(withJSONObject:fixtureView.foundation(),options:[.prettyPrinted,.sortedKeys,.withoutEscapingSlashes])
        try demand(prepared.count<=262144,"resource-limit")
        phase="seed-decode"
        let snapshot=try oracleDecode(prepared)
        phase="seed-core"
        let engine=try oracleEngine(snapshot)
        phase="capture"
        func capture(_ key:String,_ returnedID:String) throws {
            var stages:[String:J]=[:]
            for name in ["preflight","seed-prepare","seed-decode","seed-core","command","snapshot","from-domain","encode-package","encode-export","self-decode","self-encode","cross-preflight","cross-inspect","cross-decode"] {
                let na = ["from-domain","cross-inspect"].contains(name) || (key=="seed" ? name=="command" : ["seed-prepare","seed-decode","seed-core"].contains(name))
                let done = name=="preflight" || (key=="seed" ? ["seed-prepare","seed-decode","seed-core"].contains(name) : name=="command")
                stages[name] = .string(na ? "not-applicable" : done ? "success" : "not-run")
            }
            var selfEqual=false
            var current="snapshot"
            defer {
                let returned:J = returnedID.isEmpty ? .object([:]) : .object([(key=="create-one" ? "chain" : "trace"):.string(returnedID)])
                points.append(.object(["key":.string(key),"returned":returned,"stages":.object(stages),"artifacts":.array(archive.artifacts[key] ?? []),"selfEqual":.bool(selfEqual)]))
            }
            do {
                let snapshot=oracleSnapshot(engine)
                let direct=try oracleProjection(snapshot);try profile(direct)
                let upper=try conservativeBudget(direct)
                try demand(archive.count+4<=18 && archive.bytes+upper*4<=4194304,"resource-limit")
                let directBytes=try instrumentJSON(direct)
                try archive.save(key,"swift-engine-projection",directBytes)
                try profile(parseSnapshot(directBytes,entry:"swift-engine-projection"))
                stages[current] = .string("success")
                current="encode-package"
                let package=try oracleEncode(snapshot)
                try archive.save(key,"swift-package",package)
                let packageView=try parseSnapshot(package,entry:"swift-package");try profile(packageView)
                stages[current] = .string("success")
                current="encode-export"
                var exported=snapshot;exported.preferences.localFirstSyncPolicy.enabled=false
                let exportView=try oracleProjection(exported);try profile(exportView,export:true);_ = try conservativeBudget(exportView)
                let exportBytes=try oracleEncode(exported)
                try archive.save(key,"swift-export",exportBytes)
                let exportParsed=try parseSnapshot(exportBytes,entry:"swift-export");try profile(exportParsed,export:true)
                stages[current] = .string("success")
                current="self-decode"
                let bytes=package
                let decoded=try oracleDecode(bytes)
                stages[current] = .string("success")
                current="self-encode"
                _ = try conservativeBudget(oracleProjection(decoded))
                let reencoded=try oracleEncode(decoded)
                try archive.save(key,"swift-self-reencoded",reencoded)
                let selfView=try parseSnapshot(reencoded,entry:"swift-self-reencoded");try profile(selfView)
                stages[current] = .string("success");selfEqual=package==reencoded
                archive.artifacts[key]=(archive.artifacts[key] ?? []).map {artifact -> J in var a=artifact.object;a["rawState"] = .string("schema-valid");return .object(a)}
            } catch { stages[current] = .string("rejected");throw error }
        }
        try capture("seed","")
        for command in caseValue["steps"].array {
            let bits=command["nowBits"].string;try demand(bits.count==16)
            guard let bitValue=UInt64(bits,radix:16) else {throw OracleError.rejected("schema-invalid")}
            let now=Date(timeIntervalSinceReferenceDate:Double(bitPattern:bitValue))
            try demand(now.timeIntervalSinceReferenceDate.isFinite)
            let before=try oracleProjection(oracleSnapshot(engine))
            let returned:String
            if command["op"] == .string("create") {
                let selected=vectors.array.filter{$0["id"]==command["noteVectorID"]};try demand(selected.count==1)
                let note=try optionalText(selected[0]["input"])
                try predictCommand(before,command,note)
                let title=try optionalText(.object(["present":.bool(true),"utf8Hex":command["titleUTF8Hex"]]))!
                returned=try oracleCreate(engine,title:title,description:optionalText(command["description"]),note:note,now:now)
            } else {
                try demand(command["op"] == .string("schedule"));try predictCommand(before,command,nil)
                guard let uuid=UUID(uuidString:command["chainRawID"].string) else {throw OracleError.rejected("schema-invalid")}
                returned=try oracleSchedule(engine,chain:TaskChainID(uuid),date:date(command["date"]),today:date(command["today"]),now:now)
            }
            try capture(command["commandKey"].string,returned)
        }
        try archive.result(.object(["schemaVersion":.number("2"),"caseID":caseValue["id"],"fixtureValidity":.string("valid"),"points":.array(points),"interopAccepted":.bool(false)]))
        } catch {
            let validity=["seed-decode","seed-core"].contains(phase) ? "invalid-fixture" : "harness-error"
            if points.isEmpty {
                var stages:[String:J]=[:]
                for name in ["preflight","seed-prepare","seed-decode","seed-core","command","snapshot","from-domain","encode-package","encode-export","self-decode","self-encode","cross-preflight","cross-inspect","cross-decode"] {
                    let na=["command","from-domain","cross-inspect"].contains(name)
                    let completed=phase != "preflight" && (name=="preflight" || name=="seed-prepare" || (phase=="seed-core" && name=="seed-decode"))
                    stages[name] = .string(na ? "not-applicable" : name==phase ? "rejected" : completed ? "success" : "not-run")
                }
                points.append(.object(["key":.string("seed"),"returned":.object([:]),"stages":.object(stages),"artifacts":.array([]),"selfEqual":.bool(false)]))
            }
            try archive.result(.object(["schemaVersion":.number("2"),"caseID":caseID,"fixtureValidity":.string(validity),"points":.array(points),"interopAccepted":.bool(false)]))
        }
    }
}
