// SPDX-License-Identifier: AGPL-3.0-only
// Typed instrumentation only. Never used as the package serializer.
import Foundation
import NoonmarkCore
func projectID(_ v:UUID)->J { .object(["rawValue":.string(v.uuidString)]) }
func projectStamp(_ v:Date)throws->J {try demand(v.timeIntervalSinceReferenceDate.isFinite);return .number(String(v.timeIntervalSinceReferenceDate.bitPattern))}
func envelope(_ snapshot:NoonmarkSnapshot)throws->J { .object(["formatVersion":.number("7"),"snapshot":try projectSnapshot(snapshot)]) }
func projectSnapshot(_ v:NoonmarkSnapshot) throws -> J {
try preflightTypedSnapshot(v)
var o:[String:J]=[:]
try demand(v.days.count<=16,"resource-limit")
o["days"] = .array(try v.days.map {try projectDay($0)})
try demand(v.taskCycleSeries.isEmpty,"profile-violation");o["taskCycleSeries"] = .array([])
try demand(v.chains.count<=16,"resource-limit")
o["chains"] = .array(try v.chains.map {try projectChain($0)})
try demand(v.definitions.count<=16,"resource-limit")
o["definitions"] = .array(try v.definitions.map {try projectDefinition($0)})
try demand(v.traces.count<=16,"resource-limit")
o["traces"] = .array(try v.traces.map {try projectTrace($0)})
try demand(v.subtasks.count<=16,"resource-limit")
o["subtasks"] = .array(try v.subtasks.map {try projectSubtask($0)})
o["preferences"] = try projectPreferences(v.preferences)
o["classifications"] = try projectClassifications(v.classifications)
try demand(v.ideas.isEmpty,"profile-violation");o["ideas"] = .array([])
return .object(o)
}
func projectLocalDate(_ v:LocalDate) throws -> J {
var o:[String:J]=[:]
o["year"] = .number(String(v.year))
o["month"] = .number(String(v.month))
o["day"] = .number(String(v.day))
return .object(o)
}
func projectDay(_ v:Day) throws -> J {
var o:[String:J]=[:]
o["id"] = projectID(v.id.rawValue)
o["date"] = try projectLocalDate(v.date)
try demand(v.lockedAt == nil,"profile-violation") // "lockedAt"
try demand(v.reviewSummary == nil,"profile-violation") // "reviewSummary"
try demand(v.reviewUnfinishedReason == nil,"profile-violation") // "reviewUnfinishedReason"
try demand(v.reviewTomorrowNote == nil,"profile-violation") // "reviewTomorrowNote"
o["createdAt"] = try projectStamp(v.createdAt)
o["updatedAt"] = try projectStamp(v.updatedAt)
return .object(o)
}
func projectChain(_ v:TaskChain) throws -> J {
var o:[String:J]=[:]
o["id"] = projectID(v.id.rawValue)
o["state"] = .string(v.state.rawValue)
try demand(v.cycleMembership == nil,"profile-violation") // "cycleMembership"
try demand(v.noteEntries.count<=1,"resource-limit")
try demand(v.noteEntries.count<=1,"resource-limit")
o["noteEntries"] = .array(try v.noteEntries.map {try projectNote($0)})
o["createdAt"] = try projectStamp(v.createdAt)
o["updatedAt"] = try projectStamp(v.updatedAt)
return .object(o)
}
func projectNote(_ v:TaskNoteEntry) throws -> J {
var o:[String:J]=[:]
o["id"] = projectID(v.id.rawValue)
try demand(v.body.utf8.count<=4096,"resource-limit")
o["body"] = .string(v.body)
o["createdAt"] = try projectStamp(v.createdAt)
o["updatedAt"] = try projectStamp(v.updatedAt)
try demand(v.deletedAt == nil,"profile-violation") // "deletedAt"
return .object(o)
}
func projectPlannedSubtask(_ v:PlannedSubtask) throws -> J {
var o:[String:J]=[:]
o["id"] = projectID(v.id.rawValue)
o["lineageID"] = projectID(v.lineageID.rawValue)
try demand(v.title.utf8.count<=4096,"resource-limit")
try demand(v.title.utf8.count<=4096,"resource-limit")
try demand(v.title.utf8.count<=4096,"resource-limit")
o["title"] = .string(v.title)
o["difficulty"] = .number(String(v.difficulty.rawValue))
o["position"] = .number(String(v.position))
o["createdAt"] = try projectStamp(v.createdAt)
return .object(o)
}
func projectDefinition(_ v:TaskDefinition) throws -> J {
var o:[String:J]=[:]
o["id"] = projectID(v.id.rawValue)
o["chainID"] = projectID(v.chainID.rawValue)
o["sequence"] = .number(String(v.sequence))
try demand(v.title.utf8.count<=4096,"resource-limit")
try demand(v.title.utf8.count<=4096,"resource-limit")
try demand(v.title.utf8.count<=4096,"resource-limit")
o["title"] = .string(v.title)
if let x=v.descriptionText {try demand(x.utf8.count<=4096,"resource-limit");o["descriptionText"] = .string(x)}
try demand(v.plannedSubtasks.count<=2,"resource-limit")
o["plannedSubtasks"] = .array(try v.plannedSubtasks.map {try projectPlannedSubtask($0)})
o["createdAt"] = try projectStamp(v.createdAt)
o["contentUpdatedAt"] = try projectStamp(v.contentUpdatedAt)
try demand(v.supersededAt == nil,"profile-violation") // "supersededAt"
try demand(v.supersededByDefinitionID == nil,"profile-violation") // "supersededByDefinitionID"
return .object(o)
}
func projectTrace(_ v:DayTrace) throws -> J {
var o:[String:J]=[:]
o["id"] = projectID(v.id.rawValue)
o["chainID"] = projectID(v.chainID.rawValue)
o["definitionID"] = projectID(v.definitionID.rawValue)
o["date"] = try projectLocalDate(v.date)
o["status"] = .string(v.status.rawValue)
o["priority"] = .number(String(v.priority))
try demand(v.pinOrder == nil,"profile-violation") // "pinOrder"
o["continuationSeq"] = .number(String(v.continuationSeq))
if let x=v.descriptionText {try demand(x.utf8.count<=4096,"resource-limit");o["descriptionText"] = .string(x)}
try demand(v.noteEntries.count<=1,"resource-limit")
try demand(v.noteEntries.count<=1,"resource-limit")
o["noteEntries"] = .array(try v.noteEntries.map {try projectNote($0)})
try demand(v.manualProgressPercent == nil,"profile-violation") // "manualProgressPercent"
try demand(v.carriedFromTraceID == nil,"profile-violation") // "carriedFromTraceID"
try demand(v.changedToTraceID == nil,"profile-violation") // "changedToTraceID"
o["createdAt"] = try projectStamp(v.createdAt)
o["contentUpdatedAt"] = try projectStamp(v.contentUpdatedAt)
try demand(v.completedAt == nil,"profile-violation") // "completedAt"
try demand(v.settledAt == nil,"profile-violation") // "settledAt"
try demand(v.draftCancellationID == nil,"profile-violation") // "draftCancellationID"
try demand(v.draftCancelledOn == nil,"profile-violation") // "draftCancelledOn"
return .object(o)
}
func projectSubtask(_ v:Subtask) throws -> J {
var o:[String:J]=[:]
o["id"] = projectID(v.id.rawValue)
o["lineageID"] = projectID(v.lineageID.rawValue)
o["traceID"] = projectID(v.traceID.rawValue)
try demand(v.title.utf8.count<=4096,"resource-limit")
try demand(v.title.utf8.count<=4096,"resource-limit")
try demand(v.title.utf8.count<=4096,"resource-limit")
o["title"] = .string(v.title)
o["status"] = .string(v.status.rawValue)
o["difficulty"] = .number(String(v.difficulty.rawValue))
o["position"] = .number(String(v.position))
try demand(v.carriedFromSubtaskID == nil,"profile-violation") // "carriedFromSubtaskID"
o["createdAt"] = try projectStamp(v.createdAt)
o["updatedAt"] = try projectStamp(v.updatedAt)
try demand(v.completedAt == nil,"profile-violation") // "completedAt"
try demand(v.settledAt == nil,"profile-violation") // "settledAt"
try demand(v.draftCancellationID == nil,"profile-violation") // "draftCancellationID"
return .object(o)
}
func projectPreferences(_ v:AppPreferences) throws -> J {
var o:[String:J]=[:]
o["theme"] = .string(v.theme.rawValue)
o["language"] = .string(v.language.rawValue)
o["themeLanguageUpdatedAt"] = try projectStamp(v.themeLanguageUpdatedAt)
try demand(v.themeLanguageWriterID.utf8.count<=4096,"resource-limit")
o["themeLanguageWriterID"] = .string(v.themeLanguageWriterID)
o["dataMode"] = .string(v.dataMode.rawValue)
o["backupPolicy"] = try projectBackupPolicy(v.backupPolicy)
o["localFirstSyncPolicy"] = try projectSyncPolicy(v.localFirstSyncPolicy)
o["settingsPoemDisplayPolicy"] = try projectPoemPolicy(v.settingsPoemDisplayPolicy)
try demand(v.syncEndpointOptions.isEmpty,"profile-violation");o["syncEndpointOptions"] = .array([])
return .object(o)
}
func projectBackupPolicy(_ v:ScheduledBackupPolicy) throws -> J {
var o:[String:J]=[:]
o["frequency"] = .string(v.frequency.rawValue)
o["destination"] = .string(v.destination.rawValue)
return .object(o)
}
func projectSyncPolicy(_ v:LocalFirstCloudSyncPolicy) throws -> J {
var o:[String:J]=[:]
o["enabled"] = .bool(v.enabled)
o["endpoint"] = .string(v.endpoint.rawValue)
o["mode"] = .string(v.mode.rawValue)
o["intervalSeconds"] = .number(String(v.intervalSeconds))
o["generateConflictCopy"] = .bool(v.generateConflictCopy)
o["snapshotRetention"] = try projectRetentionPolicy(v.snapshotRetention)
return .object(o)
}
func projectRetentionPolicy(_ v:SyncSnapshotRetentionPolicy) throws -> J {
var o:[String:J]=[:]
o["indexRetentionDays"] = .number(String(v.indexRetentionDays))
o["retentionIndexesDaily"] = .number(String(v.retentionIndexesDaily))
return .object(o)
}
func projectPoemPolicy(_ v:SettingsPoemDisplayPolicy) throws -> J {
var o:[String:J]=[:]
o["enabled"] = .bool(v.enabled)
try demand(v.text.utf8.count<=4096,"resource-limit")
o["text"] = .string(v.text)
return .object(o)
}
func projectClassifications(_ v:TaskClassificationState) throws -> J {
var o:[String:J]=[:]
// Authenticated wire constant: upstream currentFormatVersion is private static.
// Actual package bytes independently verify this wire value.
o["formatVersion"] = .number("1")
o["revision"] = .number(String(v.revision))
try demand(v.categories.isEmpty,"profile-violation");o["categories"] = .array([])
try demand(v.labels.isEmpty,"profile-violation");o["labels"] = .array([])
try demand(v.currentByChainID.isEmpty,"profile-violation");o["currentByChainID"] = .array([])
try demand(v.relationHistory.isEmpty,"profile-violation");o["relationHistory"] = .array([])
try demand(v.categoryMerges.isEmpty,"profile-violation");o["categoryMerges"] = .array([])
try demand(v.labelMerges.isEmpty,"profile-violation");o["labelMerges"] = .array([])
try demand(v.categoryDeletionTombstones.isEmpty,"profile-violation");o["categoryDeletionTombstones"] = .array([])
try demand(v.labelDeletionTombstones.isEmpty,"profile-violation");o["labelDeletionTombstones"] = .array([])
try demand(v.snapshotsByTraceID.isEmpty,"profile-violation");o["snapshotsByTraceID"] = .array([])
try demand(v.snapshotEventsByTraceID.isEmpty,"profile-violation");o["snapshotEventsByTraceID"] = .array([])
try demand(v.committedReceiptsByInteractionID.isEmpty,"profile-violation");o["committedReceiptsByInteractionID"] = .array([])
try demand(v.changeRecords.isEmpty,"profile-violation");o["changeRecords"] = .array([])
return .object(o)
}

func preflightTypedSnapshot(_ v:NoonmarkSnapshot)throws {
    try demand(v.days.count<=16 && v.chains.count<=16 && v.definitions.count<=16 && v.traces.count<=16 && v.subtasks.count<=16,"resource-limit")
    var entities=v.days.count+v.chains.count+v.definitions.count+v.traces.count+v.subtasks.count
    var strings=0
    func text(_ s:String?)throws {if let s=s {try demand(s.utf8.count<=4096,"resource-limit");strings += s.utf8.count;try demand(strings<=65536,"resource-limit")}}
    for d in v.definitions {try demand(d.plannedSubtasks.count<=2,"resource-limit");entities += d.plannedSubtasks.count;try text(d.title);try text(d.descriptionText);for p in d.plannedSubtasks {try text(p.title)}}
    for c in v.chains {try demand(c.noteEntries.count<=1,"resource-limit");entities += c.noteEntries.count;for n in c.noteEntries {try text(n.body)}}
    for t in v.traces {try demand(t.noteEntries.count<=1,"resource-limit");entities += t.noteEntries.count;try text(t.descriptionText);for n in t.noteEntries {try text(n.body)}}
    for s in v.subtasks {try text(s.title)}
    try text(v.preferences.themeLanguageWriterID);try text(v.preferences.settingsPoemDisplayPolicy.text)
    try demand(entities<=96,"resource-limit")
}
