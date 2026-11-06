// SPDX-License-Identifier: AGPL-3.0-only
import Foundation
import Darwin
import CryptoKit

enum OracleError: Error { case rejected(String) }
func demand(_ condition: Bool, _ reason: String = "schema-invalid") throws {
    if !condition { throw OracleError.rejected(reason) }
}
indirect enum J: Equatable, Sendable {
    case object([String:J]), array([J]), string(String), number(String), bool(Bool), null
    var object: [String:J] { if case .object(let v) = self { return v }; return [:] }
    var array: [J] { if case .array(let v) = self { return v }; return [] }
    var string: String { if case .string(let v) = self { return v }; return "" }
    subscript(_ key: String) -> J { object[key] ?? .null }
    func foundation() -> Any {
        switch self {
        case .object(let v): return v.mapValues { $0.foundation() }
        case .array(let v): return v.map { $0.foundation() }
        case .string(let v): return v
        case .number(let v): return v.first == "-" ? NSNumber(value:Int64(v)!) : NSNumber(value:UInt64(v)!)
        case .bool(let v): return v
        case .null: return NSNull()
        }
    }
}
// Bounded recursive token scanner: no Foundation JSON parser, domain decode,
// reflection, cloning or serialization is reached until this has succeeded.
struct Scanner {
    let data: [UInt8]
    let maxDepth: Int, maxNodes: Int, maxString: Int, maxTotalString: Int, maxMembers: Int, maxArray: Int
    var i=0, nodes=0, strings=0
    init(_ bytes: Data, limit: Int=262144, depth: Int=24, nodes: Int=8192,
         string: Int=4096, totalString: Int=65536, members: Int=32, array: Int=16) throws {
        try demand(bytes.count <= limit,"resource-limit")
        try demand(String(data:bytes,encoding:.utf8) != nil)
        self.data=Array(bytes); self.maxDepth=depth;self.maxNodes=nodes;self.maxString=string
        self.maxTotalString=totalString;self.maxMembers=members;self.maxArray=array
    }
    mutating func whitespace() { while i<data.count && [9,10,13,32].contains(data[i]) { i += 1 } }
    mutating func countNode() throws { nodes += 1; try demand(nodes<=maxNodes,"resource-limit") }
    mutating func take(_ c:UInt8) throws { try demand(i<data.count && data[i]==c);i += 1 }
    mutating func hex4() throws -> UInt32 {
        try demand(i+4<=data.count); var value:UInt32=0
        for _ in 0..<4 { let c=data[i]; i += 1; let d:UInt32
            switch c { case 48...57:d=UInt32(c-48);case 65...70:d=UInt32(c-55);case 97...102:d=UInt32(c-87);default:throw OracleError.rejected("schema-invalid") }
            value=value*16+d
        };return value
    }
    mutating func text() throws -> String {
        try take(34);var bytes:[UInt8]=[]
        while i<data.count {
            let c=data[i];i += 1
            if c==34 {
                guard let value=String(bytes:bytes,encoding:.utf8) else {throw OracleError.rejected("schema-invalid")}
                strings += bytes.count;try demand(strings<=maxTotalString,"resource-limit");return value
            }
            try demand(c>=32)
            if c != 92 { bytes.append(c) }
            else {
                try demand(i<data.count); let e=data[i];i += 1
                switch e {
                case 34,47,92:bytes.append(e)
                case 98:bytes.append(8);case 102:bytes.append(12);case 110:bytes.append(10);case 114:bytes.append(13);case 116:bytes.append(9)
                case 117:
                    var scalar=try hex4()
                    if (0xD800...0xDBFF).contains(scalar) {
                        try take(92);try take(117);let low=try hex4();try demand((0xDC00...0xDFFF).contains(low))
                        scalar=0x10000+(scalar-0xD800)*1024+low-0xDC00
                    }
                    guard let u=UnicodeScalar(scalar) else {throw OracleError.rejected("schema-invalid")}
                    bytes.append(contentsOf:String(u).utf8)
                default:throw OracleError.rejected("schema-invalid")
                }
            };try demand(bytes.count<=maxString,"resource-limit")
        };throw OracleError.rejected("schema-invalid")
    }
    mutating func value(_ depth:Int) throws -> J {
        try demand(depth<=maxDepth,"resource-limit");try countNode();whitespace();try demand(i<data.count)
        switch data[i] {
        case 123:
            i += 1;whitespace();var object:[String:J]=[:]
            if i<data.count && data[i]==125 {i += 1;return .object(object)}
            while true {
                try demand(object.count<maxMembers,"resource-limit");try countNode();let key=try text()
                try demand(object[key]==nil);whitespace();try take(58);object[key]=try value(depth+1);whitespace();try demand(i<data.count)
                if data[i]==125 {i += 1;return .object(object)};try take(44);whitespace()
            }
        case 91:
            i += 1;whitespace();var array:[J]=[]
            if i<data.count && data[i]==93 {i += 1;return .array(array)}
            while true {try demand(array.count<maxArray,"resource-limit");array.append(try value(depth+1));whitespace();try demand(i<data.count)
                if data[i]==93 {i += 1;return .array(array)};try take(44);whitespace()}
        case 34:return .string(try text())
        case 116,102,110:
            let word=data[i]==116 ? "true" : (data[i]==102 ? "false" : "null")
            for c in word.utf8 {try take(c)}
            return word=="null" ? .null : .bool(word=="true")
        default:
            let start=i;if data[i]==45 {i += 1};try demand(i<data.count)
            if data[i]==48 {i += 1}
            else {try demand((49...57).contains(data[i]));while i<data.count && (48...57).contains(data[i]) {i += 1}}
            try demand(i-start<=20);let token=String(decoding:data[start..<i],as:UTF8.self)
            try demand(token.first=="-" ? Int64(token) != nil : UInt64(token) != nil)
            return .number(token)
        }
    }
    mutating func parse() throws -> J {let result=try value(1);whitespace();try demand(i==data.count);return result}
}
func digest(_ data:Data)->String { SHA256.hash(data:data).map{String(format:"%02x",$0)}.joined() }
func readLimited(_ path:String, _ limit:Int) throws -> Data {
    var info=stat();try demand(lstat(path,&info)==0 && (info.st_mode & mode_t(S_IFMT))==mode_t(S_IFREG),"source-mismatch")
    let handle=try FileHandle(forReadingFrom:URL(fileURLWithPath:path));defer{try? handle.close()}
    let bytes=try handle.read(upToCount:limit+1) ?? Data();try demand(bytes.count<=limit,"resource-limit");return bytes
}
func parseSnapshot(_ bytes:Data,entry:String="internal") throws -> J {
    let value=try scannerIngress(entry,bytes)
    let target=ingressTrace?.entry==entry && ingressTrace?.active==true
    if target{ingressEvent("profile")}
    do{try validateType(value,"Envelope");try demand(value["formatVersion"] == .number("7"))}catch{if target{ingressEvent("profile-reject")};throw error};return value
}
func conservativeBudget(_ value:J) throws -> Int {
    var nodes=0,strings=0
    func visit(_ v:J,_ depth:Int) throws {
        try demand(depth<=24,"resource-limit");nodes += 1;try demand(nodes<=8192,"resource-limit")
        switch v {
        case .object(let o):try demand(o.count<=32,"resource-limit");for(k,v) in o {nodes += 1;strings += k.utf8.count;try visit(v,depth+1)}
        case .array(let a):try demand(a.count<=16,"resource-limit");for v in a {try visit(v,depth+1)}
        case .string(let s):try demand(s.utf8.count<=4096,"resource-limit");strings += s.utf8.count
        default:break
        };try demand(strings<=65536 && nodes<=8192,"resource-limit")
    }
    try visit(value,1);let upper=128*nodes+6*strings+2;try demand(upper<=262144,"resource-limit");return upper
}
