// SPDX-License-Identifier: AGPL-3.0-only
// 实际调用精确上游模块；这里没有规范化／分段算法的替代实现。
import Darwin
import Foundation
import NoonmarkCore

struct VectorFile: Decodable {
    let schemaVersion: Int
    let provenance: String
    let cases: [Vector]
    let pairs: [Pair]
}
struct Vector: Decodable { let id: String; let input: String }
struct Pair: Codable { let id: String; let left: String; let right: String }
struct CharacterRecord: Encodable {
    let startScalar: Int
    let endScalar: Int
    let isWhitespace: Bool
}
struct TextRecord: Encodable {
    let text: String
    let utf8Hex: String
    let unicodeScalars: [UInt32]
    let characterCount: Int
    let characters: [CharacterRecord]
    init(_ text: String) {
        self.text = text
        utf8Hex = text.utf8.map { String(format: "%02x", $0) }.joined()
        unicodeScalars = text.unicodeScalars.map(\.value)
        var offset = 0
        var records: [CharacterRecord] = []
        for character in text {
            let end = offset + character.unicodeScalars.count
            records.append(CharacterRecord(startScalar: offset, endScalar: end,
                                           isWhitespace: character.isWhitespace))
            offset = end
        }
        characterCount = records.count
        characters = records
    }
}
struct CaseRecord: Encodable {
    let id: String
    let input: TextRecord
    let displayName: TextRecord
    let canonicalKey: TextRecord
    let displayNameOfDisplayName: TextRecord
    let canonicalKeyOfDisplayName: TextRecord
    init(_ vector: Vector) {
        id = vector.id
        input = TextRecord(vector.input)
        let display = ClassificationNameCanonicalizer.displayName(vector.input)
        displayName = TextRecord(display)
        canonicalKey = TextRecord(ClassificationNameCanonicalizer.canonicalKey(vector.input))
        displayNameOfDisplayName = TextRecord(ClassificationNameCanonicalizer.displayName(display))
        canonicalKeyOfDisplayName = TextRecord(ClassificationNameCanonicalizer.canonicalKey(display))
    }
}
struct Equality: Encodable {
    let swiftEqual: Bool
    let utf8Equal: Bool
    init(_ left: String, _ right: String) {
        swiftEqual = left == right
        utf8Equal = left.utf8.elementsEqual(right.utf8)
    }
}
struct PairRecord: Encodable {
    let id: String
    let left: String
    let right: String
    let input: Equality
    let displayName: Equality
    let canonicalKey: Equality
    init(_ pair: Pair, _ a: CaseRecord, _ b: CaseRecord) {
        id = pair.id; left = pair.left; right = pair.right
        input = Equality(a.input.text, b.input.text)
        displayName = Equality(a.displayName.text, b.displayName.text)
        canonicalKey = Equality(a.canonicalKey.text, b.canonicalKey.text)
    }
}
struct Evidence: Encodable {
    let schemaVersion: Int
    let algorithmVersion: String
    let icuVersion: String
    let icuUnicodeVersion: String
    let cases: [CaseRecord]
    let pairs: [PairRecord]
    let entityRoundTrips: [EntityRecord]
}

struct EntityRecord: Encodable {
    let caseID: String
    let entity: String
    let name: TextRecord
    let canonicalKey: TextRecord
    let canonicalKeyVersion: String
    let encodedByteCount: Int
    let decodeStatus: String
    let decodeErrorKind: String?
    let decodeErrorDescription: String?
    let roundTripEqual: Bool?
    let nameUTF8Equal: Bool?
    let keyUTF8Equal: Bool?
}
func entityRecord<T: Codable & Equatable>(_ value: T, id: String, kind: String,
    name: String, key: String, version: String,
    extract: (T) -> (String, String)) throws -> EntityRecord {
    let data = try JSONEncoder().encode(value)
    do {
        let decoded = try JSONDecoder().decode(T.self, from: data)
        let fields = extract(decoded)
        return EntityRecord(caseID: id, entity: kind, name: TextRecord(name),
            canonicalKey: TextRecord(key), canonicalKeyVersion: version,
            encodedByteCount: data.count, decodeStatus: "accepted", decodeErrorKind: nil,
            decodeErrorDescription: nil, roundTripEqual: decoded == value,
            nameUTF8Equal: name.utf8.elementsEqual(fields.0.utf8),
            keyUTF8Equal: key.utf8.elementsEqual(fields.1.utf8))
    } catch {
        let errorKind: String
        let description: String?
        switch error {
        case DecodingError.dataCorrupted(let context):
            errorKind = "dataCorrupted"; description = String(context.debugDescription.prefix(256))
        case DecodingError.keyNotFound: errorKind = "keyNotFound"; description = nil
        case DecodingError.typeMismatch: errorKind = "typeMismatch"; description = nil
        case DecodingError.valueNotFound: errorKind = "valueNotFound"; description = nil
        default: errorKind = "other"; description = nil
        }
        return EntityRecord(caseID: id, entity: kind, name: TextRecord(name),
            canonicalKey: TextRecord(key), canonicalKeyVersion: version,
            encodedByteCount: data.count, decodeStatus: "rejected", decodeErrorKind: errorKind,
            decodeErrorDescription: description, roundTripEqual: nil,
            nameUTF8Equal: nil, keyUTF8Equal: nil)
    }
}

enum OracleError: Error { case invalidInvocation, invalidInput, missingSymbol, outputLimit }
func icuVersion(_ name: String) throws -> String {
    typealias GetVersion = @convention(c) (UnsafeMutablePointer<UInt8>) -> Void
    guard let symbol = dlsym(UnsafeMutableRawPointer(bitPattern: -2), name) else {
        throw OracleError.missingSymbol
    }
    let function = unsafeBitCast(symbol, to: GetVersion.self)
    var version: [UInt8] = [0, 0, 0, 0]
    version.withUnsafeMutableBufferPointer { function($0.baseAddress!) }
    return version.map(String.init).joined(separator: ".")
}

guard CommandLine.arguments.count == 1 else { throw OracleError.invalidInvocation }
// 输入只来自调用器校验过的固定合成向量；不接受文件路径、网络或环境选项。
var bytes = Data()
while true {
    let chunk = FileHandle.standardInput.readData(ofLength: min(65536, 2 * 1024 * 1024 + 1 - bytes.count))
    if chunk.isEmpty { break }
    bytes.append(chunk)
    guard bytes.count <= 2 * 1024 * 1024 else { throw OracleError.invalidInput }
}
let vectors = try JSONDecoder().decode(VectorFile.self, from: bytes)
guard vectors.schemaVersion == 1, vectors.provenance == "fixed-synthetic-only",
      !vectors.cases.isEmpty, vectors.cases.count <= 128, vectors.pairs.count <= 128,
      Set(vectors.cases.map(\.id)).count == vectors.cases.count,
      vectors.cases.allSatisfy({ $0.input.utf8.count <= 1024 * 1024 }) else {
    throw OracleError.invalidInput
}
let records = vectors.cases.map(CaseRecord.init)
let index = Dictionary(uniqueKeysWithValues: records.map { ($0.id, $0) })
var pairs: [PairRecord] = []
for pair in vectors.pairs {
    guard let left = index[pair.left], let right = index[pair.right] else {
        throw OracleError.invalidInput
    }
    pairs.append(PairRecord(pair, left, right))
}
// 固定实体 ID、时间和颜色；上游初始化器自建历史 UUID，不回写也不输出该随机值。
let syntheticID = UUID(uuidString: "00000000-0000-4000-8000-000000000001")!
let syntheticTime = Date(timeIntervalSince1970: 1_700_000_000)
let roundTripIDs: Set<String> = ["ascii", "empty", "only-di", "display-non-idempotent", "space-mark", "compat-diaeresis"]
var entityRecords: [EntityRecord] = []
for vector in vectors.cases where roundTripIDs.contains(vector.id) {
    let category = TaskCategory(id: TaskCategoryID(syntheticID), name: vector.input,
        colorHex: "#336699", now: syntheticTime)
    entityRecords.append(try entityRecord(category, id: vector.id, kind: "TaskCategory",
        name: category.name, key: category.canonicalKey, version: category.canonicalKeyVersion,
        extract: { ($0.name, $0.canonicalKey) }))
    let label = TaskLabel(id: TaskLabelID(syntheticID), name: vector.input,
        colorHex: "#336699", now: syntheticTime)
    entityRecords.append(try entityRecord(label, id: vector.id, kind: "TaskLabel",
        name: label.name, key: label.canonicalKey, version: label.canonicalKeyVersion,
        extract: { ($0.name, $0.canonicalKey) }))
}
let evidence = Evidence(schemaVersion: 1,
    algorithmVersion: ClassificationNameCanonicalizer.algorithmVersion,
    icuVersion: try icuVersion("u_getVersion"),
    icuUnicodeVersion: try icuVersion("u_getUnicodeVersion"), cases: records, pairs: pairs,
    entityRoundTrips: entityRecords)
let encoder = JSONEncoder()
encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
let output = try encoder.encode(evidence)
guard output.count <= 16 * 1024 * 1024 else { throw OracleError.outputLimit }
FileHandle.standardOutput.write(output)
