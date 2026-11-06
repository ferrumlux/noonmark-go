// SPDX-License-Identifier: AGPL-3.0-only
// Closed synthetic mutations only. These do not add business cases or points.
import Foundation
import NoonmarkCore
import NoonmarkStorage

let mutationIDs:Set<String> = [
    "control-valid",
    "raw-bytes","raw-depth","raw-nodes","raw-string","raw-string-sum","raw-array","raw-members",
    "profile-empty-array","profile-pairs-odd","profile-pairs-duplicate","profile-pairs-nonempty","profile-base64",
    "hex-odd","hex-invalid-utf8","hex-decoded-plus-one","hex-double-expansion",
    "create-growth-nodes","create-growth-entities","create-growth-text","create-growth-output",
    "schedule-growth-nodes","schedule-growth-entities","schedule-growth-text","schedule-growth-output"
]
func mutationJSON(_ v:J)throws->Data {
    // Only the bounded, fixed generator calls Foundation here. This is not the
    // target preflight/parser and does not encode a domain entity or package.
    try JSONSerialization.data(withJSONObject:v.foundation(),options:[.fragmentsAllowed,.sortedKeys])
}
func replaceSnapshot(_ envelope:J,_ field:String,_ value:J)->J {
    var root=envelope.object;var snapshot=envelope["snapshot"].object;snapshot[field]=value;root["snapshot"] = .object(snapshot);return .object(root)
}
func runMutation(_ id:String,spec:String,output:String)throws {
    try demand(mutationIDs.contains(id),"registry-mismatch")
    let archive=try Archive(output)
    operationWitnessCounts=["decode":0,"clone":0,"bridge":0,"command":0,"serializer":0]
    var reason="none",accepted=false
    do {
        var registryParser=try Scanner(Data(registryJSON.utf8),limit:65536,depth:12,nodes:4096,string:8192,totalString:32768,members:32,array:18)
        let registry=try registryParser.parse()
        let seedID=id.hasPrefix("schedule-") ? "planned-12" : "empty"
        let row=registry["seeds"].array.first{$0["id"] == .string(seedID)}!
        let original=try readLimited(URL(fileURLWithPath:spec).appendingPathComponent(row["file"].string).path,262144)
        try demand(digest(original)==row["sha256"].string,"source-mismatch")
        let originalView=try parseSnapshot(original);try profile(originalView)
        var view=originalView;var raw=original
        var checkRaw=true
        switch id {
        case "control-valid":checkRaw=false
        case "raw-bytes":raw=Data(repeating:32,count:262145)
        case "raw-depth":raw=Data((String(repeating:"[",count:25)+"0"+String(repeating:"]",count:25)).utf8)
        case "raw-nodes":
            let leaves=J.array(Array(repeating:.number("0"),count:16))
            let mid=J.array(Array(repeating:leaves,count:16));raw=try mutationJSON(.array(Array(repeating:.array(Array(repeating:mid,count:16)),count:2)))
        case "raw-string":raw=try mutationJSON(.string(String(repeating:"x",count:4097)))
        case "raw-string-sum":
            let row=J.array(Array(repeating:.string(String(repeating:"x",count:4096)),count:9));raw=try mutationJSON(.array([row,row]))
        case "raw-array":raw=try mutationJSON(.array(Array(repeating:.number("0"),count:17)))
        case "raw-members":raw=try mutationJSON(.object(Dictionary(uniqueKeysWithValues:(0..<33).map{("f"+String($0),J.number("0"))})))
        case "profile-empty-array":view=replaceSnapshot(view,"ideas",.array([.object([:])]));raw=try mutationJSON(view)
        case "profile-pairs-odd","profile-pairs-duplicate","profile-pairs-nonempty":
            var classifications=view["snapshot"]["classifications"].object
            let key=J.object(["rawValue":.string("00000000-0000-4000-8000-000000000001")])
            let pair:[J]=[key,.object([:])]
            classifications["categories"] = .array(id=="profile-pairs-odd" ? [key] : id=="profile-pairs-duplicate" ? pair+pair : pair)
            view=replaceSnapshot(view,"classifications",.object(classifications));raw=try mutationJSON(view)
        case "profile-base64":var object=view.object;object["base64"] = .string("e30=");view = .object(object);raw=try mutationJSON(view)
        case "hex-odd","hex-invalid-utf8","hex-decoded-plus-one","hex-double-expansion":
            checkRaw=false
            let hex=id=="hex-odd" ? "0" : id=="hex-invalid-utf8" ? "ff" : String(repeating:"61",count:id=="hex-decoded-plus-one" ? 4097 : 8192)
            _ = try optionalText(.object(["present":.bool(true),"utf8Hex":.string(hex)]))
        default:
            checkRaw=false
            let op=id.hasPrefix("create-") ? "create" : "schedule"
            let caseID=op=="create" ? "create-valid" : "schedule-planned-12"
            let command=registry["cases"].array.first{$0["id"] == .string(caseID)}!["steps"].array[0]
            var note:String?=nil
            if id.hasSuffix("-entities") {
                let field=op=="create" ? "chains" : "traces"
                view=replaceSnapshot(view,field,.array(Array(repeating:.object([:]),count:16)))
            } else if id.hasSuffix("-text") {
                if op=="create" {note=String(repeating:"x",count:4097)}
                else {
                    var definitions=view["snapshot"]["definitions"].array
                    var definition=definitions[0].object;var planned=definition["plannedSubtasks"]!.array
                    var child=planned[0].object;child["title"] = .string(String(repeating:"x",count:4097))
                    planned[0] = .object(child);definition["plannedSubtasks"] = .array(planned);definitions[0] = .object(definition)
                    view=replaceSnapshot(view,"definitions",.array(definitions))
                }
            }
            else if id.hasSuffix("-nodes") {
                let leaves=J.array(Array(repeating:.number("0"),count:16))
                let mid=J.array(Array(repeating:leaves,count:16))
                view=replaceSnapshot(view,"ideas",.array(Array(repeating:.array(Array(repeating:mid,count:16)),count:2)))
            } else {
                view=replaceSnapshot(view,"ideas",.array(Array(repeating:.string(String(repeating:"x",count:4096)),count:12)))
            }
            try predictCommand(view,command,note)
        }
        if checkRaw {let parsed=try parseSnapshot(raw);try profile(parsed)}
        // If a supposedly rejected input passes preflight, the real downstream
        // operation is counted and attempted. A post-decoder failure is NOT a
        // passing preflight negative. No zero-counter assertion can mask it.
        let prepared=try JSONSerialization.data(withJSONObject:view.foundation(),options:[.prettyPrinted,.sortedKeys,.withoutEscapingSlashes])
        let decoded=try oracleDecode(checkRaw ? raw : prepared)
        let initial=try oracleEngine(decoded)
        let engine=oracleClone(initial)
        if id.hasPrefix("schedule-") {
            let command=registry["cases"].array.first{$0["id"] == .string("schedule-planned-12")}!["steps"].array[0]
            guard let owner=UUID(uuidString:command["chainRawID"].string)else{throw OracleError.rejected("schema-invalid")}
            _ = try oracleSchedule(engine,chain:TaskChainID(owner),date:date(command["date"]),today:date(command["today"]),now:Date(timeIntervalSinceReferenceDate:101))
        }else{_ = try oracleCreate(engine,title:"Synthetic negative gate",description:nil,note:nil,now:Date(timeIntervalSinceReferenceDate:100))}
        let snapshot=oracleSnapshot(engine)
        let projected=try oracleProjection(snapshot);try profile(projected);_ = try conservativeBudget(projected)
        _ = try oracleEncode(snapshot)
        accepted=true
    } catch {
        if case OracleError.rejected(let r)=error {reason=r} else {reason="domain-rejected"}
    }
    try archive.result(.object(["schemaVersion":.number("2"),"id":.string(id),"accepted":.bool(accepted),"reason":.string(reason),"counters":.object(operationWitnessCounts.mapValues{.number(String($0))})]))
}

// Shared actual-ingress instrumentation. No caller payloads or adjustable limits.
final class IngressTrace {
    let entry:String,dimension:String,mode:String
    var reached=false,active=false,rejected=false
    var inputBytes=0,inputSHA256=""
    var events:[String]=[]
    var counters:[String:Int]=["materialize":0,"decode":0,"clone":0,"bridge":0,"command":0,"serializer":0]
    init(_ e:String,_ d:String,_ m:String){entry=e;dimension=d;mode=m}
}
nonisolated(unsafe) var operationWitnessCounts:[String:Int]=["decode":0,"clone":0,"bridge":0,"command":0,"serializer":0]
nonisolated(unsafe) var ingressTrace:IngressTrace? = nil
nonisolated(unsafe) var ingressTemplate:J = .null
nonisolated(unsafe) var ingressMulti:J = .null
func ingressEvent(_ event:String){if operationWitnessCounts[event] != nil {operationWitnessCounts[event,default:0] += 1};guard let t=ingressTrace,t.active else{return};if t.events.count<64{t.events.append(event)};if t.counters[event] != nil{t.counters[event,default:0] += 1}}
struct IngressLimits {
    var bytes=262144,depth=24,nodes=8192,string=4096,sum=65536,members=32,array=16
}
func ingressLimits(_ entry:String)->IngressLimits {
    switch entry {
    case "config-FieldSchema-R2.json":return IngressLimits(bytes:65536,depth:8,nodes:4096,string:256,sum:49152,members:32,array:19)
    case "config-SeedRegistry-R2.json":return IngressLimits(bytes:65536,depth:12,nodes:4096,string:8192,sum:32768,members:32,array:18)
    case "config-NoteVectors-R2.json":return IngressLimits(bytes:16384,depth:6,nodes:256,string:8192,sum:16384,members:8,array:11)
    case "config-case":return IngressLimits(bytes:16384,depth:12,nodes:4096,string:8192,sum:32768,members:32,array:18)
    default:return IngressLimits()
    }
}
func scannerIngress(_ entry:String,_ original:Data)throws->J {
    let l=ingressLimits(entry)
    let target=ingressTrace?.entry==entry && ingressTrace?.reached==false
    var data=original,mode="normal"
    if target,let t=ingressTrace {t.reached=true;t.active=true;mode=t.mode;if t.dimension != "positive"{data=try ingressMutation(t.dimension,l,original)}}
    if target,let t=ingressTrace{t.inputBytes=data.count;t.inputSHA256=digest(data)}
    func scan()throws->J {
        if target{ingressEvent("scan")}
        do {var p=try Scanner(data,limit:l.bytes,depth:l.depth,nodes:l.nodes,string:l.string,totalString:l.sum,members:l.members,array:l.array)
            let value=try p.parse();if target{ingressEvent("scan-pass")};return value
        }catch{if target{ingressEvent("reject");ingressTrace?.rejected=true};throw error}
    }
    func materialize()throws {if target{ingressEvent("materialize")};_ = try JSONSerialization.jsonObject(with:data,options:[.fragmentsAllowed])}
    if mode=="normal"{let value=try scan();try materialize();if entry.hasPrefix("config-"){try demand(digest(data)==digest(original),"registry-mismatch")};return value}
    try materialize()
    if mode=="late"{return try scan()}
    // In bypass fault mode, actual Foundation decoding happened without the
    // shared scanner. Returning the frozen input permits downstream witnesses;
    // no malformed input can become a business observation in this matrix mode.
    var originalScanner=try Scanner(original,limit:l.bytes,depth:l.depth,nodes:l.nodes,string:l.string,totalString:l.sum,members:l.members,array:l.array)
    return try originalScanner.parse()
}
func oracleDecode(_ data:Data)throws->NoonmarkSnapshot {ingressEvent("decode");return try NoonmarkDataPackage.decode(data)}
func oracleEngine(_ snapshot:NoonmarkSnapshot)throws->NoonmarkEngine {ingressEvent("clone");return try NoonmarkEngine(snapshot:snapshot)}
func oracleClone(_ engine:NoonmarkEngine)->NoonmarkEngine {ingressEvent("clone");return NoonmarkEngine(copying:engine)}
func oracleSnapshot(_ engine:NoonmarkEngine)->NoonmarkSnapshot {ingressEvent("clone");return engine.snapshot()}
func oracleProjection(_ snapshot:NoonmarkSnapshot)throws->J {ingressEvent("bridge");return try envelope(snapshot)}
func oracleEncode(_ snapshot:NoonmarkSnapshot)throws->Data {ingressEvent("serializer");return try NoonmarkDataPackage.encode(snapshot)}
func oracleCreate(_ engine:NoonmarkEngine,title:String,description:String?,note:String?,now:Date)throws->String {ingressEvent("command");return try engine.createPoolTask(title:title,descriptionText:description,initialNoteBody:note,now:now).rawValue.uuidString}
func oracleSchedule(_ engine:NoonmarkEngine,chain:TaskChainID,date:LocalDate,today:LocalDate,now:Date)throws->String {ingressEvent("command");return try engine.scheduleFromPool(chainID:chain,date:date,today:today,now:now).rawValue.uuidString}

func ingressMutation(_ dim:String,_ l:IngressLimits,_ original:Data)throws->Data {
    switch dim {
    case "bytes":return Data(repeating:32,count:l.bytes+1)
    case "depth":return Data((String(repeating:"[",count:l.depth)+"0"+String(repeating:"]",count:l.depth)).utf8)
    case "string":return try mutationJSON(.string(String(repeating:"x",count:l.string+1)))
    case "sum":
        var a=Array(repeating:J.string(String(repeating:"x",count:l.string)),count:l.sum/l.string+1)
        if a.count>l.array{var rows:[J]=[];while !a.isEmpty{let n=min(a.count,l.array);rows.append(.array(Array(a.prefix(n))));a.removeFirst(n)};return try mutationJSON(.array(rows))};return try mutationJSON(.array(a))
    case "array":return try mutationJSON(.array(Array(repeating:.bool(true),count:l.array+1)))
    case "members":return try mutationJSON(.object(Dictionary(uniqueKeysWithValues:(0...l.members).map{("k"+String($0),J.bool(true))})))
    case "nodes":
        func tree(_ n:Int)->J {if n<=1{return .bool(true)};var rest=n;let chunk=(n+l.array-1)/l.array;var a:[J]=[];while rest>0{let k=min(rest,chunk);a.append(tree(k));rest -= k};return .array(a)}
        return try mutationJSON(tree(l.nodes+1))
    case "utf8":return Data([255])
    case "duplicate":return Data("{\"x\":1,\"x\":2}".utf8)
    case "float":return Data("{\"x\":1.25}".utf8)
    default:break
    }
    var p=try Scanner(original,limit:l.bytes,depth:l.depth,nodes:l.nodes,string:l.string,totalString:l.sum,members:l.members,array:l.array)
    var v=try p.parse()
    if dim=="cardinality" {
        if case .array(var a)=v{a.append(a[0]);v = .array(a)}else{var m=v.object;if let seedList=m["seeds"],case .array(var a)=seedList{a.append(a[0]);m["seeds"] = .array(a)}else if let objectMap=m["objects"],case .object(var o)=objectMap{o.removeValue(forKey:"Envelope");m["objects"] = .object(o)}else{m["unknown"] = .bool(true)};v = .object(m)}
    }else if dim.hasPrefix("hex-"){
        var a=v.array;var row=a[0].object;var input=row["input"]!.object
        let h=dim=="hex-odd" ? "0" : dim=="hex-invalid-utf8" ? "ff" : String(repeating:"61",count:dim=="hex-decoded-plus-one" ? 4097 : 8192)
        input["present"] = .bool(true);input["utf8Hex"] = .string(h);row["input"] = .object(input);a[0] = .object(row);v = .array(a)
    }else {
        var root=v.object;var s=v["snapshot"].object
        switch dim {
        case "empty-array":s["ideas"] = .array([.bool(true)])
        case "base64":root["base64"] = .string("AA==")
        case "pairs-odd","pairs-duplicate","pairs-nonempty":var c=s["classifications"]!.object;c["currentByChainID"] = .array(Array(repeating:.string("duplicate"),count:dim=="pairs-odd" ? 1 : dim=="pairs-duplicate" ? 4 : 2));s["classifications"] = .object(c)
        case "days-array","definitions-array","traces-array","subtasks-array":s[String(dim.dropLast(6))] = .array(Array(repeating:.object([:]),count:17))
        case "trace-notes-array":var t=ingressMulti["snapshot"]["traces"].array[0].object;t["noteEntries"] = .array([.object([:]),.object([:])]);s["traces"] = .array([.object(t)])
        case "entities-array":s["chains"] = .array(Array(repeating:ingressTemplate["snapshot"]["chains"].array[0],count:17))
        case "planned-array":var d=ingressTemplate["snapshot"]["definitions"].array[0].object;d["plannedSubtasks"] = .array(Array(repeating:d["plannedSubtasks"]!.array[0],count:3));s["definitions"] = .array([.object(d)])
        case "notes-array":var c=ingressTemplate["snapshot"]["chains"].array[0].object;c["noteEntries"] = .array([.object([:]),.object([:])]);s["chains"] = .array([.object(c)])
        default:throw OracleError.rejected("unknown-dimension")
        };root["snapshot"] = .object(s);v = .object(root)
    };return try mutationJSON(v)
}

let ingressEntries=["config-FieldSchema-R2.json","config-SeedRegistry-R2.json","config-NoteVectors-R2.json","config-case","seed-empty","seed-planned-12","seed-planned-21","seed-multi-interleaved","swift-engine-projection","swift-package","swift-export","swift-self-reencoded","cross"]
func ingressDimensions(_ entry:String)->[String]{
    var d=["positive","bytes","depth","nodes","string","sum","array","members","utf8","duplicate","float"]
    if entry.hasPrefix("config-"){d.append("cardinality");if entry=="config-NoteVectors-R2.json"{d += ["hex-odd","hex-invalid-utf8","hex-decoded-plus-one","hex-double-expansion"]}}
    else{d += ["empty-array","pairs-odd","pairs-duplicate","pairs-nonempty","base64","entities-array","planned-array","notes-array","days-array","definitions-array","traces-array","subtasks-array","trace-notes-array"]};return d
}
func runIngressMatrix(_ mode:String,spec:String,output:String)throws {
    try demand(["normal","bypass","late"].contains(mode),"unknown-matrix-mode")
    let archive=try Archive(output)
    defer{ingressTrace=nil}
    let plannedBytes=try readLimited(URL(fileURLWithPath:spec).appendingPathComponent("r2-seeds/planned-12.json").path,262144)
    try demand(digest(plannedBytes)=="aecba81054e72fbdf749c928cbd5d8346c856c258e3e718f30a6f0ff71c2decb","source-mismatch")
    ingressTemplate=try parseSnapshot(plannedBytes)
    let multiBytes=try readLimited(URL(fileURLWithPath:spec).appendingPathComponent("r2-seeds/multi-interleaved.json").path,262144)
    try demand(digest(multiBytes)=="4e3159d88491d994a2410972513c6312258609d8ce3ad89aceed28c5585120da","source-mismatch")
    ingressMulti=try parseSnapshot(multiBytes)
    var rows:[J]=[],expected:[J]=[],pass=true,ordinal=0
    for entry in ingressEntries {for dim in ingressDimensions(entry){
        ordinal += 1;let trace=IngressTrace(entry,dim,mode);ingressTrace=trace
        var accepted=false,reason=""
        do{accepted=try dispatchIngress(entry,spec:spec,root:archive.root.appendingPathComponent("work-"+String(ordinal)).path)}catch{if case OracleError.rejected(let r)=error{reason=r}else{reason="domain-rejected"}}
        if !accepted && reason.isEmpty{reason=trace.rejected ? "preflight-rejected" : "downstream-rejected"}
        var ok=false
        if dim=="positive"{ok=trace.reached && accepted && trace.counters["materialize",default:0]>0 && Array(trace.events.prefix(2))==["scan","scan-pass"]}
        else {
            let schemaDimension=dim=="cardinality" || dim.hasPrefix("hex-") || dim.contains("pairs-") || dim=="empty-array" || dim=="base64" || dim.hasSuffix("-array")
            let zero=trace.counters.allSatisfy{(schemaDimension && $0.key=="materialize") || $0.value==0}
            ok=trace.reached && !accepted && zero && trace.events.first=="scan" && (schemaDimension ? trace.events.count>=2 : trace.events==["scan","reject"])
        }
        pass = pass && ok;expected.append(.string(entry+"/"+dim))
        rows.append(.object(["inputBytes":.number(String(trace.inputBytes)),"inputSHA256":.string(trace.inputSHA256),"entry":.string(entry),"dimension":.string(dim),"outcome":.string(accepted ? "accepted" : "rejected"),"reason":.string(reason),"reached":.bool(trace.reached),"events":.array(trace.events.map{.string($0)}),"counters":.object(trace.counters.mapValues{.number(String($0))}),"pass":.bool(ok)]))
    }}
    for op in ["create","schedule"]{
        let trace=IngressTrace(op,"positive",mode);trace.active=true;trace.reached=true;ingressTrace=trace
        var accepted=false,reason=""
        do{accepted=try dispatchIngress(op,spec:spec,root:archive.root.appendingPathComponent("control-"+op).path)}catch{reason="domain-rejected"}
        let ok=accepted && ["decode","clone","bridge","command","serializer"].allSatisfy{trace.counters[$0,default:0]>0}
        pass=pass && ok;expected.append(.string(op+"/positive"))
        rows.append(.object(["inputBytes":.number(String(trace.inputBytes)),"inputSHA256":.string(trace.inputSHA256),"entry":.string(op),"dimension":.string("positive"),"outcome":.string(accepted ? "accepted" : "rejected"),"reason":.string(reason),"reached":.bool(true),"events":.array(trace.events.map{.string($0)}),"counters":.object(trace.counters.mapValues{.number(String($0))}),"pass":.bool(ok)]))
    }
    try archive.result(.object(["schemaVersion":.number("2"),"mode":.string(mode),"pass":.bool(pass),"expectedRows":.array(expected),"rows":.array(rows)]))
}
func dispatchIngress(_ entry:String,spec:String,root:String)throws->Bool {
    try FileManager.default.createDirectory(atPath:root,withIntermediateDirectories:false)
    var registryParser=try Scanner(Data(registryJSON.utf8),limit:65536,depth:12,nodes:4096,string:8192,totalString:32768,members:32,array:18)
    let registry=try registryParser.parse()
    let caseID=entry=="create" ? "create-valid" : entry=="schedule" ? "schedule-planned-12" : entry.hasPrefix("seed-") ? "observe-"+String(entry.dropFirst(5)) : "observe-empty"
    guard let selected=registry["cases"].array.first(where:{$0["id"].string==caseID}),let seed=registry["seeds"].array.first(where:{$0["id"]==selected["seed"]})else{throw OracleError.rejected("registry-mismatch")}
    let casePath=URL(fileURLWithPath:root).appendingPathComponent("case.json")
    let caseBytes=try mutationJSON(selected)
    if entry=="create" || entry=="schedule"{ingressTrace?.inputBytes=caseBytes.count;ingressTrace?.inputSHA256=digest(caseBytes)}
    try caseBytes.write(to:casePath,options:.withoutOverwriting)
    let resultRoot=URL(fileURLWithPath:root).appendingPathComponent("capture").path
    try SnapshotOracle.run(["--fixture":URL(fileURLWithPath:spec).appendingPathComponent(seed["file"].string).path,"--case":casePath.path,"--vectors":URL(fileURLWithPath:spec).appendingPathComponent("NoteVectors-R2.json").path,"--spec":spec,"--output":resultRoot])
    var resultParser=try Scanner(readLimited(URL(fileURLWithPath:resultRoot).appendingPathComponent("result.json").path,131072),limit:131072,depth:16,nodes:8192,string:4096,totalString:65536,members:32,array:64)
    let result=try resultParser.parse();guard result["fixtureValidity"] == .string("valid")else{return false}
    if entry=="cross" {
        let destination=URL(fileURLWithPath:root).appendingPathComponent("cross-result").path
        try SnapshotOracle.run(["--cross":URL(fileURLWithPath:resultRoot).appendingPathComponent("seed/swift-package.json").path,"--spec":spec,"--output":destination])
        var parser=try Scanner(readLimited(URL(fileURLWithPath:destination).appendingPathComponent("result.json").path,131072));return try parser.parse()["accepted"] == .bool(true)
    };return true
}
