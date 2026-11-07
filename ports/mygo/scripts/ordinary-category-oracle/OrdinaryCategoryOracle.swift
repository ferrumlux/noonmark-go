// SPDX-License-Identifier: AGPL-3.0-only
// Reference-only fixture producer for unchanged quboliu/noonmark@43e6eef.
// No product admission, snapshot suite, storage, UI, AI or platform integration.
@testable import NoonmarkCore
import CryptoKit
import Darwin
import Foundation

private let inputSHA = "2a3f43ffb5a78165ce9d32958089e0e3973280829fa5f3ab33f69fa44ea70aae"
private let suite = "ordinary-category-only-v1"
private enum HarnessFailure: Error { case invalid(String) }
private func require(_ yes: Bool, _ why: String) throws {
    if !yes { throw HarnessFailure.invalid(why) }
}
private func sha(_ bytes: Data) -> String {
    SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
}
private func encoder() -> JSONEncoder {
    let e = JSONEncoder(); e.outputFormatting = [.sortedKeys]; return e
}
private func bits(_ date: Date) -> String {
    String(format: "%016llx", date.timeIntervalSinceReferenceDate.bitPattern)
}
private func text(_ s: String) -> [String: Any] {
    ["utf8Base64": Data(s.utf8).base64EncodedString(), "text": s]
}
private func failure(_ e: Error) -> [String: Any] {
    ["status": "error", "errorType": String(reflecting: type(of: e)),
     "message": String(String(describing: e).prefix(2048))]
}
private func ok() -> [String: Any] { ["status": "ok"] }
private let positiveControls: Set<String> = [
    "main-new", "main-repeat", "main-retry-original", "main-retry-changed-now", "main-none", "main-none-again",
    "chain-b-first-none-earlier-global-time", "chain-b-existing", "chain-b-new-name-reuse",
    "same-base-first-positive", "tamper-positive-control"
]
private func controls(_ id: String) -> [String] {
    switch id {
    case "main-repeat", "relation-clock-backward": return ["main-new"]
    case "main-retry-original", "main-retry-changed-now", "retry-changed-decision": return ["main-new", "main-repeat"]
    case "main-none": return ["main-repeat"]
    case "main-none-again": return ["main-none"]
    case "chain-b-first-none-earlier-global-time": return ["main-none-again"]
    case "chain-b-existing": return ["main-new", "chain-b-first-none-earlier-global-time"]
    case "chain-b-new-name-reuse": return ["chain-b-existing"]
    case "same-base-first-positive", "tamper-positive-control": return ["chain-b-new-name-reuse"]
    case "same-base-stale-second": return ["same-base-first-positive"]
    case "preview-tamper-old-digest", "preview-tamper-rehashed", "digest-tamper": return ["tamper-positive-control"]
    case "missing-removal", "removal-origin-decision", "removal-time-backward", "removal-revision-not-later", "audit-order-reversed": return ["main-none"]
    default: return []
    }
}
private func notRun(_ why: String) -> [String: Any] { ["status": "not-run", "reason": why] }

private struct Input: Decodable {
    struct Name: Decodable { let id: String; let raw: String }
    let schema: String; let suite: String; let titles: [String]
    let chainClockBits: String; let clocks: [String: String]
    let interactionIDs: [UUID]; let names: [Name]
    func date(_ name: String) throws -> Date {
        guard let value = clocks[name], let b = UInt64(value, radix: 16) else {
            throw HarnessFailure.invalid("unknown frozen clock")
        }
        return Date(timeIntervalSinceReferenceDate: Double(bitPattern: b))
    }
}

// EXACT Encodable field material at TC:806-824, independently reconstructed.
// These bytes are NOT intercepted from the private original encoder.
private struct ReconstructedPlanPayload: Encodable {
    struct Label: Encodable { let canonicalKey: String; let id: TaskLabelID }
    let formatVersion: Int = 1
    let planID: UUID; let baseRevision: UInt64; let interactionID: UUID
    let source: ClassificationSource; let changes: [ClassificationPlanChange]
    let createdItemID: UUID?; let plannedCategoryID: TaskCategoryID?
    let plannedLabelIDs: [Label]
    let notices: [ClassificationNotice]; let blockers: [ClassificationBlocker]
    init(_ p: ClassificationPlan) throws {
        try require(p.plannedCreations.labelIDsByCanonicalKey.isEmpty, "labels outside scope")
        planID = p.id; baseRevision = p.baseRevision; interactionID = p.interactionID
        source = p.source; changes = p.changes; createdItemID = p.createdItemID
        plannedCategoryID = p.plannedCreations.categoryID; plannedLabelIDs = []
        notices = p.notices; blockers = p.blockers
    }
}
// EXACT material at TC:989-1004, date is UInt64 bits, never Date JSON here.
private struct ReconstructedRecordPayload: Encodable {
    let formatVersion: Int = 1
    let id: UUID; let planID: UUID; let interactionID: UUID
    let source: ClassificationSource; let decisionID: UUID?
    let changes: [ClassificationPlanChange]; let notices: [ClassificationNotice]
    let committedAtBitPattern: UInt64; let revision: UInt64; let planDigest: String
    init(_ r: ClassificationChangeRecord) {
        id = r.id; planID = r.planID; interactionID = r.interactionID
        source = r.source; decisionID = r.decisionID; changes = r.changes
        notices = r.notices; committedAtBitPattern = r.committedAt.timeIntervalSinceReferenceDate.bitPattern
        revision = r.revision; planDigest = r.planDigest
    }
}

private final class Archive {
    let root: URL
    var files: [[String: Any]] = []
    var observations: [String] = []
    private var bytes = 0
    init(_ path: String) throws {
        root = URL(fileURLWithPath: path, isDirectory: true)
        let values = try root.resourceValues(forKeys: [.isDirectoryKey, .isSymbolicLinkKey])
        try require(values.isDirectory == true && values.isSymbolicLink != true, "output must be real directory")
        try require(try FileManager.default.contentsOfDirectory(atPath: path).isEmpty, "output must be empty")
    }
    func save(_ name: String, _ data: Data, role: String, cap: Int = 262_144) throws -> [String: Any] {
        try require(!name.isEmpty && name.utf8.count <= 120 && name.allSatisfy { $0.isASCII && ($0.isLetter || $0.isNumber || "-._".contains($0)) }, "bad output filename")
        try require(data.count <= cap && files.count < 512 && data.count <= 16_777_216 - bytes, "artifact budget")
        try data.write(to: root.appendingPathComponent(name), options: .withoutOverwriting)
        let ref: [String: Any] = ["path": name, "bytes": data.count, "sha256": sha(data), "role": role]
        bytes += data.count; files.append(ref); return ref
    }
    func encoded<T: Encodable>(_ name: String, _ value: T, role: String) throws -> [String: Any] {
        try save(name, encoder().encode(value), role: role)
    }
    func metadata(_ id: String, _ value: [String: Any]) throws {
        var v = value; v["schema"] = "ordinary-category-observation-v1"; v["caseID"] = id
        _ = try save(id + ".json", JSONSerialization.data(withJSONObject: v, options: [.sortedKeys]), role: "observation", cap: 65_536)
        observations.append(id)
    }
    func finish() throws {
        let v: [String: Any] = ["schema": "ordinary-category-artifacts-v1", "suite": suite,
            "upstreamCommit": "43e6eefaae3a04be89cab3e234d470111afd80ef", "inputSHA256": inputSHA,
            "captureComplete": true, "semanticParityVerified": false, "runtimeAdmission": false,
            "fullSnapshotExecuted": false, "causalIDNormalizationApplied": false,
            "algorithmVersion": ClassificationNameCanonicalizer.algorithmVersion,
            "observationIDs": observations, "artifactBytes": bytes, "files": files]
        let d = try JSONSerialization.data(withJSONObject: v, options: [.sortedKeys])
        try require(d.count <= 131_072, "manifest budget")
        try d.write(to: root.appendingPathComponent("manifest.json"), options: .withoutOverwriting)
    }
}

private func choiceView(_ choice: TaskCategoryChoice?) -> [String: Any] {
    guard let choice else { return ["kind": "none"] }
    switch choice {
    case .existing(let id): return ["kind": "existing", "categoryID": id.description]
    case .new(let name, let color): return ["kind": "new", "name": text(name), "colorHex": text(color)]
    }
}
private func intentView(_ intent: ClassificationIntent) throws -> [String: Any] {
    switch intent {
    case .setCurrent(let d):
        try require(d.labels.isEmpty, "labels outside scope")
        return ["kind": "setCurrent", "chainID": d.chainID.description, "category": choiceView(d.category), "labels": []]
    case .createCategory(let name, let color):
        return ["kind": "createCategory", "name": text(name), "colorHex": text(color), "referenceOnly": true]
    default: throw HarnessFailure.invalid("intent outside frozen reference scope")
    }
}
private func dateFacts(_ s: TaskClassificationState) -> [String: String] {
    var v: [String: String] = [:]
    for (id, c) in s.categories {
        let p = "category/" + id.description
        v[p + "/createdAt"] = bits(c.createdAt); v[p + "/updatedAt"] = bits(c.updatedAt)
        for n in c.nameVersions {
            v[p + "/nameVersion/" + n.id.uuidString + "/validFrom"] = bits(n.validFrom)
            if let d = n.validUntil { v[p + "/nameVersion/" + n.id.uuidString + "/validUntil"] = bits(d) }
        }
    }
    for (id, c) in s.currentByChainID {
        if let r = c.category {
            v["current/" + id.description + "/createdAt"] = bits(r.createdAt)
            v["current/" + id.description + "/updatedAt"] = bits(r.updatedAt)
        }
    }
    for r in s.relationHistory {
        v["removal/" + r.id.uuidString + "/createdAt"] = bits(r.createdAt)
        v["removal/" + r.id.uuidString + "/removedAt"] = bits(r.removedAt)
    }
    for r in s.changeRecords { v["record/" + r.id.uuidString + "/committedAt"] = bits(r.committedAt) }
    return v
}
private func stateView(_ s: TaskClassificationState) -> [String: Any] {
    let categories = s.categories.values.sorted { $0.id.description < $1.id.description }.map { c -> [String: Any] in
        ["id": c.id.description, "name": text(c.name), "canonicalKey": text(c.canonicalKey),
         "canonicalKeyVersion": c.canonicalKeyVersion, "nameVersionIDs": c.nameVersions.map { $0.id.uuidString }]
    }
    return ["revision": String(s.revision), "categories": categories,
        "currentKeys": s.currentByChainID.keys.map(\.description).sorted(),
        "emptyCurrentKeys": s.currentByChainID.filter { $0.value.category == nil }.keys.map(\.description).sorted(),
        "recordIDsInOriginalOrder": s.changeRecords.map { $0.id.uuidString },
        "removalIDsInOriginalOrder": s.relationHistory.map { $0.id.uuidString },
        "receiptInteractionIDs": s.committedReceiptsByInteractionID.keys.map(\.uuidString).sorted(),
        "dateBitsHex": dateFacts(s)]
}

private final class Oracle {
    let input: Input; let archive: Archive
    var interaction = 0
    var passedControls: Set<String> = []
    init(input: Input, archive: Archive) { self.input = input; self.archive = archive }
    func id() throws -> UUID {
        try require(interaction < input.interactionIDs.count, "interaction budget")
        defer { interaction += 1 }; return input.interactionIDs[interaction]
    }
    func seed(_ prefix: String) throws -> (NoonmarkEngine, TaskChainID, TaskChainID) {
        let e = NoonmarkEngine()
        guard let b = UInt64(input.chainClockBits, radix: 16) else { throw HarnessFailure.invalid("chain clock") }
        let now = Date(timeIntervalSinceReferenceDate: Double(bitPattern: b))
        let a = try e.createPoolTask(title: input.titles[0], now: now)
        let bID = try e.createPoolTask(title: input.titles[1], now: now)
        // Graph arrays only, not NoonmarkSnapshot Codable or full snapshot validation.
        struct Graph: Encodable { let chains: [TaskChain]; let definitions: [TaskDefinition] }
        _ = try archive.encoded(prefix + "-graph.json", Graph(chains: e.chains.values.sorted { $0.id.description < $1.id.description }, definitions: e.definitions.values.sorted { $0.id.description < $1.id.description }), role: "actual-synthetic-ordinary-graph")
        return (e, a, bID)
    }
    func stages(_ s: TaskClassificationState, _ caseID: String) throws -> [String: Any] {
        var v: [String: Any] = ["state": stateView(s)]
        do { try s.validateIntegrity(); v["integrity"] = ok() } catch { v["integrity"] = failure(error) }
        let raw: Data
        do { raw = try encoder().encode(s) }
        catch { v["encode"] = failure(error); v["decode"] = notRun("encode failed"); return v }
        // Archive raw encoder result before attempting decode, without reserialization.
        v["encode"] = ["status": "ok", "originalBytes": try archive.save(caseID + "-state.json", raw, role: "actual-Foundation-state-Codable-bytes"), "dateStrategy": "deferredToDate"]
        do {
            let decoded = try JSONDecoder().decode(TaskClassificationState.self, from: raw)
            v["decode"] = ["status": "ok", "stateEqual": decoded == s,
                "allDateBitsEqual": dateFacts(decoded) == dateFacts(s), "decodedDateBitsHex": dateFacts(decoded)]
        } catch { v["decode"] = failure(error) }
        return v
    }
    func planView(_ p: ClassificationPlan, _ name: String, intentionallyCorrupted: Bool = false) throws -> [String: Any] {
        let raw = try encoder().encode(ReconstructedPlanPayload(p))
        let match = sha(raw) == p.digest
        let originalValid = try p.hasValidDigest()
        if !intentionallyCorrupted { try require(match && originalValid, "reconstructed plan payload mismatch") }
        return ["planID": p.id.uuidString, "interactionID": p.interactionID.uuidString,
            "preparedIntent": try intentView(p.intent), "baseRevision": String(p.baseRevision),
            "digest": p.digest, "reconstructedSHA256": sha(raw), "matchesActualDigest": match,
            "originalHasValidDigest": originalValid, "intentionallyCorrupted": intentionallyCorrupted,
            "reconstructedPayloadBytes": try archive.save(name + "-plan-payload.json", raw, role: "reconstructed-plan-payload-not-intercepted")]
    }
    func recordView(_ r: ClassificationChangeRecord, _ name: String) throws -> [String: Any] {
        let raw = try encoder().encode(ReconstructedRecordPayload(r))
        try require(sha(raw) == r.integrityDigest && r.hasValidIntegrityDigest(), "reconstructed record payload mismatch")
        return ["recordID": r.id.uuidString, "integrityDigest": r.integrityDigest,
            "committedAtBitsHex": bits(r.committedAt), "matchesActualDigest": true,
            "reconstructedPayloadBytes": try archive.save(name + "-record-payload.json", raw, role: "reconstructed-record-payload-not-intercepted"),
            "originalRecordBytes": try archive.encoded(name + "-record.json", r, role: "actual-Foundation-record-Codable-bytes")]
    }
    @discardableResult
    func commit(_ e: NoonmarkEngine, caseID: String, plan p: ClassificationPlan, now: Date,
                decision: UUID? = nil, prepare: [String: Any] = ok(), request: [String: Any] = [:],
                intentionallyCorrupted: Bool = false) throws -> ClassificationReceipt? {
        let before = e.classificationState
        let controlRefs = controls(caseID)
        try require(controlRefs.allSatisfy { passedControls.contains($0) }, "dependent probe has no successful positive control")
        var v: [String: Any] = ["kind": "command", "controlRefs": controlRefs, "requiredPositiveControl": positiveControls.contains(caseID), "request": request,
            "nowBitsHex": bits(now), "prepare": prepare, "before": stateView(before),
            "originalBeforeBytes": try archive.encoded(caseID + "-before.json", before, role: "actual-before-state-bytes"),
            "plan": try planView(p, caseID, intentionallyCorrupted: intentionallyCorrupted)]
        let d = decision ?? p.interactionID
        let confirmation = ClassificationConfirmation.user(confirming: p, decisionID: d)
        v["confirmation"] = ["authority": "bound-user", "decisionID": d.uuidString,
            "planID": p.id.uuidString, "interactionID": p.interactionID.uuidString, "digest": p.digest,
            "originalAuthorizes": confirmation.authorizes(p)]
        var receipt: ClassificationReceipt?
        do { receipt = try e.commitClassification(p, confirmation: confirmation, now: now); v["commit"] = ok() }
        catch { v["commit"] = failure(error) }
        if let receipt {
            v["receiptBytes"] = try archive.encoded(caseID + "-receipt.json", receipt, role: "actual-Foundation-receipt-bytes")
            guard let record = e.classificationState.changeRecords.first(where: { $0.id == receipt.changeRecordID }) else {
                throw HarnessFailure.invalid("actual receipt missing record")
            }
            v["record"] = try recordView(record, caseID)
            v["exactReceiptAlreadyPresent"] = before.committedReceiptsByInteractionID[p.interactionID] == receipt
        }
        v["logicalAndDateBitsUnchanged"] = before == e.classificationState && dateFacts(before) == dateFacts(e.classificationState)
        let after = try stages(e.classificationState, caseID)
        v["afterStages"] = after
        try archive.metadata(caseID, v)
        if positiveControls.contains(caseID) {
            let successful = receipt != nil && ["integrity", "encode", "decode"].allSatisfy {
                (after[$0] as? [String: Any])?["status"] as? String == "ok"
            }
            // Preserve all actual error/byte evidence before failing the control.
            try require(successful, "required positive control failed: " + caseID)
            passedControls.insert(caseID)
        }
        return receipt
    }
    @discardableResult
    func command(_ e: NoonmarkEngine, caseID: String, intent: ClassificationIntent, now: Date) throws -> ClassificationPlan? {
        let interactionID = try id(); let before = e.classificationState
        let request: [String: Any] = ["intent": try intentView(intent), "interactionID": interactionID.uuidString, "prepareNowBitsHex": bits(now)]
        let p: ClassificationPlan
        do { p = try e.prepareClassification(intent, source: .userDirect, interactionID: interactionID, now: now) }
        catch {
            try archive.metadata(caseID, ["kind": "command", "request": request, "prepare": failure(error),
                "commit": notRun("prepare failed"), "logicalAndDateBitsUnchanged": e.classificationState == before && dateFacts(e.classificationState) == dateFacts(before),
                "controlRefs": controls(caseID), "requiredPositiveControl": positiveControls.contains(caseID),
                "afterStages": try stages(e.classificationState, caseID)])
            try require(!positiveControls.contains(caseID), "required positive prepare failed: " + caseID)
            return nil
        }
        try require(e.classificationState == before, "prepare mutated state")
        _ = try commit(e, caseID: caseID, plan: p, now: now, request: request)
        return p
    }
    func intent(_ chain: TaskChainID, _ category: TaskCategoryChoice?) -> ClassificationIntent {
        .setCurrent(TaskClassificationDraft(chainID: chain, category: category, labels: []))
    }
    func stateMutation(_ caseID: String, positive: TaskClassificationState, mutate: (inout TaskClassificationState) -> Void) throws {
        try require(passedControls.contains("main-none"), "state mutation positive control failed")
        var changed = positive; mutate(&changed)
        try archive.metadata(caseID, ["kind": "single-field-state-mutation", "prepare": notRun("state-only mutation"),
            "commit": notRun("state-only mutation"), "positiveControlReference": "main-none", "controlRefs": ["main-none"],
            "before": stateView(positive), "afterStages": try stages(changed, caseID)])
    }
    func ordinarySequence() throws {
        let (e, a, b) = try seed("main")
        guard let original = try command(e, caseID: "main-new", intent: intent(a, .new(name: "工程", colorHex: "#2A6FDB")), now: input.date("new")),
              let category = e.classificationState.currentByChainID[a]?.categoryID else { throw HarnessFailure.invalid("main new failed") }
        let initial = NoonmarkEngine(copying: e)
        _ = try command(e, caseID: "main-repeat", intent: intent(a, .new(name: "工程", colorHex: "#2A6FDB")), now: input.date("repeat"))
        _ = try commit(e, caseID: "main-retry-original", plan: original, now: input.date("new"), prepare: notRun("exact retained original plan"), request: ["retainedPlanCaseID": "main-new"])
        _ = try commit(e, caseID: "main-retry-changed-now", plan: original, now: input.date("retryChanged"), prepare: notRun("exact retained original plan"), request: ["retainedPlanCaseID": "main-new"])
        _ = try command(e, caseID: "main-none", intent: intent(a, nil), now: input.date("remove"))
        let removed = e.classificationState
        _ = try command(e, caseID: "main-none-again", intent: intent(a, nil), now: input.date("noneAgain"))
        _ = try command(e, caseID: "chain-b-first-none-earlier-global-time", intent: intent(b, nil), now: input.date("chainBNone"))
        _ = try command(e, caseID: "chain-b-existing", intent: intent(b, .existing(category)), now: input.date("chainBExisting"))
        _ = try command(e, caseID: "chain-b-new-name-reuse", intent: intent(b, .new(name: "工程", colorHex: "#FFFFFF")), now: input.date("chainBReuse"))
        _ = try command(NoonmarkEngine(copying: initial), caseID: "relation-clock-backward", intent: intent(a, nil), now: input.date("backward"))
        _ = try commit(NoonmarkEngine(copying: e), caseID: "retry-changed-decision", plan: original, now: input.date("retryChanged"), decision: id(), prepare: notRun("retained original plan; changed decision"))
        let branch = NoonmarkEngine(copying: e)
        let oneIntent = intent(a, nil)
        let one = try branch.prepareClassification(oneIntent, source: .userDirect, interactionID: id(), now: input.date("staleFirst"))
        let twoIntent = intent(b, nil)
        let two = try branch.prepareClassification(twoIntent, source: .userDirect, interactionID: id(), now: input.date("staleSecond"))
        _ = try commit(branch, caseID: "same-base-first-positive", plan: one, now: input.date("staleFirst"), request: ["intent": try intentView(oneIntent), "interactionID": one.interactionID.uuidString, "prepareNowBitsHex": bits(input.date("staleFirst"))])
        _ = try commit(branch, caseID: "same-base-stale-second", plan: two, now: input.date("staleSecond"), request: ["intent": try intentView(twoIntent), "interactionID": two.interactionID.uuidString, "prepareNowBitsHex": bits(input.date("staleSecond"))])
        // Fresh valid base plan, same fields except explicit mutation under test.
        let positive = try e.prepareClassification(intent(b, nil), source: .userDirect, interactionID: id(), now: input.date("staleFirst"))
        _ = try commit(NoonmarkEngine(copying: e), caseID: "tamper-positive-control", plan: positive, now: input.date("staleFirst"), request: ["intent": try intentView(intent(b, nil)), "interactionID": positive.interactionID.uuidString, "prepareNowBitsHex": bits(input.date("staleFirst"))])
        guard let first = positive.changes.first, case let .setCurrent(chain, before, _) = first else { throw HarnessFailure.invalid("setCurrent change missing") }
        let wrong = ClassificationSelectionPreview(category: ClassificationPlanItem(id: category.description, kind: .category, name: "changed preview", colorHex: "#2A6FDB", resolution: .existing), labels: [])
        let changes: [ClassificationPlanChange] = [.setCurrent(chainID: chain, before: before, after: wrong)]
        func altered(_ c: [ClassificationPlanChange], _ digest: String?) throws -> ClassificationPlan {
            try ClassificationPlan(id: positive.id, baseRevision: positive.baseRevision, interactionID: positive.interactionID,
                intent: positive.intent, source: positive.source, changes: c, createdItemID: positive.createdItemID,
                plannedCreations: positive.plannedCreations, notices: positive.notices, blockers: positive.blockers, digest: digest)
        }
        _ = try commit(NoonmarkEngine(copying: e), caseID: "preview-tamper-old-digest", plan: altered(changes, positive.digest), now: input.date("staleFirst"), prepare: notRun("post-prepare mutation of tamper-positive-control"), intentionallyCorrupted: true)
        _ = try commit(NoonmarkEngine(copying: e), caseID: "preview-tamper-rehashed", plan: altered(changes, nil), now: input.date("staleFirst"), prepare: notRun("post-prepare mutation of tamper-positive-control; digest recomputed by actual plan initializer"))
        _ = try commit(NoonmarkEngine(copying: e), caseID: "digest-tamper", plan: altered(positive.changes, String(repeating: "0", count: 64)), now: input.date("staleFirst"), prepare: notRun("post-prepare mutation of tamper-positive-control"), intentionallyCorrupted: true)
        try stateMutation("missing-removal", positive: removed) { $0.relationHistory.removeAll() }
        guard let r = removed.relationHistory.first else { throw HarnessFailure.invalid("positive removal missing") }
        let otherDecision = try id()
        func replacement(origin: UUID?, removedAt: Date, removedRevision: UInt64) -> ClassificationRelationHistoryEntry {
            ClassificationRelationHistoryEntry(id: r.id, kind: r.kind, chainID: r.chainID, itemID: r.itemID,
                originSource: r.originSource, originDecisionID: origin, createdAt: r.createdAt,
                createdRevision: r.createdRevision, removedBySource: r.removedBySource,
                removedByDecisionID: r.removedByDecisionID, removedAt: removedAt, removedRevision: removedRevision)
        }
        try stateMutation("removal-origin-decision", positive: removed) { $0.relationHistory[0] = replacement(origin: otherDecision, removedAt: r.removedAt, removedRevision: r.removedRevision) }
        try stateMutation("removal-time-backward", positive: removed) { $0.relationHistory[0] = replacement(origin: r.originDecisionID, removedAt: r.createdAt.addingTimeInterval(-1), removedRevision: r.removedRevision) }
        try stateMutation("removal-revision-not-later", positive: removed) { $0.relationHistory[0] = replacement(origin: r.originDecisionID, removedAt: r.removedAt, removedRevision: r.createdRevision) }
        try stateMutation("audit-order-reversed", positive: removed) { $0.changeRecords.reverse() }
    }
    func normalization() throws {
        for vector in input.names {
            let base = "name-" + vector.id
            var current = vector.raw
            var normalization: [[String: Any]] = []
            for i in 0...5 {
                normalization.append(["stage": "D" + String(i), "name": text(current), "KOfThisStage": text(ClassificationNameCanonicalizer.canonicalKey(current))])
                current = ClassificationNameCanonicalizer.displayName(current)
            }
            let now = try input.date("submillisecond")
            let category = TaskCategory(name: vector.raw, colorHex: "#2A6FDB", now: now)
            let raw = try encoder().encode(category)
            var direct: [String: Any] = ["kind": "normalization-reference", "entry": "direct-initializer",
                "raw": text(vector.raw), "DAndKStages": normalization, "name": text(category.name),
                "canonicalKey": text(category.canonicalKey), "originalCategoryID": category.id.description,
                "originalNameVersionIDs": category.nameVersions.map { $0.id.uuidString },
                "nowBitsHex": bits(now), "encode": ["status": "ok", "originalBytes": try archive.save(base + "-direct-category.json", raw, role: "actual-category-Codable-bytes")]]
            do {
                let decoded = try JSONDecoder().decode(TaskCategory.self, from: raw)
                direct["decode"] = ["status": "ok", "name": text(decoded.name), "createdAtBitsHex": bits(decoded.createdAt)]
            } catch { direct["decode"] = failure(error) }
            try archive.metadata(base + "-direct", direct)
            let standalone = NoonmarkEngine()
            _ = try command(standalone, caseID: base + "-prepared-create-reference", intent: .createCategory(name: vector.raw, colorHex: "#2A6FDB"), now: now)
            let (core, chain, _) = try seed(base)
            let ui = NoonmarkEngine(copying: core)
            _ = try command(core, caseID: base + "-core-new", intent: intent(chain, .new(name: vector.raw, colorHex: "#2A6FDB")), now: now)
            _ = try command(ui, caseID: base + "-ui-reference-input", intent: intent(chain, .new(name: ClassificationNameCanonicalizer.displayName(vector.raw), colorHex: "#2A6FDB")), now: now)
        }
        for clock in ["negativeZero", "zero", "submillisecond"] {
            let (e, a, _) = try seed("clock-" + clock)
            _ = try command(e, caseID: "clock-" + clock, intent: intent(a, .new(name: "时间", colorHex: "#2A6FDB")), now: input.date(clock))
        }
    }
    func run() throws { try ordinarySequence(); try normalization(); try archive.finish() }
}

@main
private enum Main {
    static func main() {
        var output: URL?
        do {
            try require(CommandLine.arguments.count == 3, "usage: ordinary-category-oracle INPUT.json EMPTY_OUTPUT_DIRECTORY")
            let inputURL = URL(fileURLWithPath: CommandLine.arguments[1])
            let handle = try FileHandle(forReadingFrom: inputURL); defer { try? handle.close() }
            let raw = try handle.read(upToCount: 16_385) ?? Data()
            try require(raw.count <= 16_384 && sha(raw) == inputSHA, "input differs from exact frozen manifest")
            let input = try JSONDecoder().decode(Input.self, from: raw)
            try require(input.schema == "ordinary-category-input-v1" && input.suite == suite && input.titles.count == 2 && input.names.count == 10, "input scope")
            let archive = try Archive(CommandLine.arguments[2]); output = archive.root
            _ = try archive.save("input-original.json", raw, role: "frozen-original-input", cap: 16_384)
            try Oracle(input: input, archive: archive).run()
            FileHandle.standardOutput.write(Data("ordinary-category capture complete; parity and runtime admission remain unverified\n".utf8))
        } catch {
            let fail: [String: Any] = ["schema": "ordinary-category-failure-v1", "captureComplete": false,
                "semanticParityVerified": false, "error": failure(error)]
            if let output, let raw = try? JSONSerialization.data(withJSONObject: fail, options: [.sortedKeys]), raw.count <= 4096 {
                try? raw.write(to: output.appendingPathComponent("failure.json"), options: .withoutOverwriting)
            }
            FileHandle.standardError.write(Data(("ordinary-category harness failure: " + String(String(describing: error).prefix(2048)) + "\n").utf8))
            exit(1)
        }
    }
}
