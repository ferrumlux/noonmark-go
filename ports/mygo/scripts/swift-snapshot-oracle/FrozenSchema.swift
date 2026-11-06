// SPDX-License-Identifier: AGPL-3.0-only
// Frozen R3 117-field schema. Generated from FieldSchema-R2.json; not from captures.
import Foundation
struct Field: Sendable { let name:String; let type:String; let optional:Bool; let profile:String }
let fields:[String:[Field]] = [
"Envelope": [
Field(name:"formatVersion",type:"int",optional:false,profile:"typed-value"),
Field(name:"snapshot",type:"Snapshot",optional:false,profile:"typed-value"),
],
"Snapshot": [
Field(name:"days",type:"[]Day",optional:false,profile:"typed-value"),
Field(name:"taskCycleSeries",type:"[]Series",optional:false,profile:"empty-array-only"),
Field(name:"chains",type:"[]Chain",optional:false,profile:"typed-value"),
Field(name:"definitions",type:"[]Definition",optional:false,profile:"typed-value"),
Field(name:"traces",type:"[]Trace",optional:false,profile:"typed-value"),
Field(name:"subtasks",type:"[]Subtask",optional:false,profile:"typed-value"),
Field(name:"preferences",type:"Preferences",optional:false,profile:"typed-value"),
Field(name:"classifications",type:"Classifications",optional:false,profile:"typed-value"),
Field(name:"ideas",type:"[]Idea",optional:false,profile:"empty-array-only"),
],
"ID": [
Field(name:"rawValue",type:"uuid",optional:false,profile:"typed-value"),
],
"LocalDate": [
Field(name:"year",type:"int",optional:false,profile:"typed-value"),
Field(name:"month",type:"int",optional:false,profile:"typed-value"),
Field(name:"day",type:"int",optional:false,profile:"typed-value"),
],
"Day": [
Field(name:"id",type:"ID",optional:false,profile:"typed-value"),
Field(name:"date",type:"LocalDate",optional:false,profile:"typed-value"),
Field(name:"lockedAt",type:"stamp",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"reviewSummary",type:"string",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"reviewUnfinishedReason",type:"string",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"reviewTomorrowNote",type:"string",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"createdAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"updatedAt",type:"stamp",optional:false,profile:"typed-value"),
],
"Chain": [
Field(name:"id",type:"ID",optional:false,profile:"typed-value"),
Field(name:"state",type:"enum:active,abandoned",optional:false,profile:"active"),
Field(name:"cycleMembership",type:"Membership",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"noteEntries",type:"[]Note",optional:false,profile:"typed-value"),
Field(name:"createdAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"updatedAt",type:"stamp",optional:false,profile:"typed-value"),
],
"Note": [
Field(name:"id",type:"ID",optional:false,profile:"typed-value"),
Field(name:"body",type:"string",optional:false,profile:"typed-value"),
Field(name:"createdAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"updatedAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"deletedAt",type:"stamp",optional:true,profile:"absent-only"),
],
"PlannedSubtask": [
Field(name:"id",type:"ID",optional:false,profile:"typed-value"),
Field(name:"lineageID",type:"ID",optional:false,profile:"typed-value"),
Field(name:"title",type:"string",optional:false,profile:"typed-value"),
Field(name:"difficulty",type:"enumint:1,2,3",optional:false,profile:"typed-value"),
Field(name:"position",type:"int",optional:false,profile:"typed-value"),
Field(name:"createdAt",type:"stamp",optional:false,profile:"typed-value"),
],
"Definition": [
Field(name:"id",type:"ID",optional:false,profile:"typed-value"),
Field(name:"chainID",type:"ID",optional:false,profile:"typed-value"),
Field(name:"sequence",type:"int",optional:false,profile:"1"),
Field(name:"title",type:"string",optional:false,profile:"typed-value"),
Field(name:"descriptionText",type:"string",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"plannedSubtasks",type:"[]PlannedSubtask",optional:false,profile:"typed-value"),
Field(name:"createdAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"contentUpdatedAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"supersededAt",type:"stamp",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"supersededByDefinitionID",type:"ID",optional:true,profile:"absent-only except descriptionText may be absent or string"),
],
"Trace": [
Field(name:"id",type:"ID",optional:false,profile:"typed-value"),
Field(name:"chainID",type:"ID",optional:false,profile:"typed-value"),
Field(name:"definitionID",type:"ID",optional:false,profile:"typed-value"),
Field(name:"date",type:"LocalDate",optional:false,profile:"typed-value"),
Field(name:"status",type:"TraceStatus",optional:false,profile:"pending"),
Field(name:"priority",type:"int",optional:false,profile:"typed-value"),
Field(name:"pinOrder",type:"int",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"continuationSeq",type:"int",optional:false,profile:"0"),
Field(name:"descriptionText",type:"string",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"noteEntries",type:"[]Note",optional:false,profile:"typed-value"),
Field(name:"manualProgressPercent",type:"int",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"carriedFromTraceID",type:"ID",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"changedToTraceID",type:"ID",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"createdAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"contentUpdatedAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"completedAt",type:"stamp",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"settledAt",type:"stamp",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"draftCancellationID",type:"uuid",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"draftCancelledOn",type:"LocalDate",optional:true,profile:"absent-only except descriptionText may be absent or string"),
],
"Subtask": [
Field(name:"id",type:"ID",optional:false,profile:"typed-value"),
Field(name:"lineageID",type:"ID",optional:false,profile:"typed-value"),
Field(name:"traceID",type:"ID",optional:false,profile:"typed-value"),
Field(name:"title",type:"string",optional:false,profile:"typed-value"),
Field(name:"status",type:"enum:pending,completed,unfinished,deferred,abandoned,cancelledDraft",optional:false,profile:"pending"),
Field(name:"difficulty",type:"enumint:1,2,3",optional:false,profile:"typed-value"),
Field(name:"position",type:"int",optional:false,profile:"typed-value"),
Field(name:"carriedFromSubtaskID",type:"ID",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"createdAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"updatedAt",type:"stamp",optional:false,profile:"typed-value"),
Field(name:"completedAt",type:"stamp",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"settledAt",type:"stamp",optional:true,profile:"absent-only except descriptionText may be absent or string"),
Field(name:"draftCancellationID",type:"uuid",optional:true,profile:"absent-only except descriptionText may be absent or string"),
],
"Preferences": [
Field(name:"theme",type:"enum:coolGray,warmPaper",optional:false,profile:"exact registry value"),
Field(name:"language",type:"enum:chinese,english",optional:false,profile:"exact registry value"),
Field(name:"themeLanguageUpdatedAt",type:"stamp",optional:false,profile:"exact registry value"),
Field(name:"themeLanguageWriterID",type:"string",optional:false,profile:"exact registry value"),
Field(name:"dataMode",type:"enum:localFirst,onlineFirst",optional:false,profile:"exact registry value"),
Field(name:"backupPolicy",type:"BackupPolicy",optional:false,profile:"exact registry value"),
Field(name:"localFirstSyncPolicy",type:"SyncPolicy",optional:false,profile:"exact registry value"),
Field(name:"settingsPoemDisplayPolicy",type:"PoemPolicy",optional:false,profile:"exact registry value"),
Field(name:"syncEndpointOptions",type:"[]SyncEndpoint",optional:false,profile:"empty-array-only"),
],
"BackupPolicy": [
Field(name:"frequency",type:"enum:off,daily,weekly",optional:false,profile:"exact registry value"),
Field(name:"destination",type:"enum:iCloudDrive,s3",optional:false,profile:"exact registry value"),
],
"SyncPolicy": [
Field(name:"enabled",type:"bool",optional:false,profile:"exact registry value"),
Field(name:"endpoint",type:"enum:iCloud,s3,webDAV,localFolder",optional:false,profile:"exact registry value"),
Field(name:"mode",type:"enum:manual,automatic",optional:false,profile:"exact registry value"),
Field(name:"intervalSeconds",type:"int",optional:false,profile:"exact registry value"),
Field(name:"generateConflictCopy",type:"bool",optional:false,profile:"exact registry value"),
Field(name:"snapshotRetention",type:"RetentionPolicy",optional:false,profile:"exact registry value"),
],
"RetentionPolicy": [
Field(name:"indexRetentionDays",type:"int",optional:false,profile:"exact registry value"),
Field(name:"retentionIndexesDaily",type:"int",optional:false,profile:"exact registry value"),
],
"PoemPolicy": [
Field(name:"enabled",type:"bool",optional:false,profile:"exact registry value"),
Field(name:"text",type:"string",optional:false,profile:"exact registry value"),
],
"Classifications": [
Field(name:"formatVersion",type:"int",optional:false,profile:"1"),
Field(name:"revision",type:"uint",optional:false,profile:"0"),
Field(name:"categories",type:"pairs:ID:Category",optional:false,profile:"empty-array-only"),
Field(name:"labels",type:"pairs:ID:Label",optional:false,profile:"empty-array-only"),
Field(name:"currentByChainID",type:"pairs:ID:Current",optional:false,profile:"empty-array-only"),
Field(name:"relationHistory",type:"[]RelationHistory",optional:false,profile:"empty-array-only"),
Field(name:"categoryMerges",type:"pairs:ID:Merge",optional:false,profile:"empty-array-only"),
Field(name:"labelMerges",type:"pairs:ID:Merge",optional:false,profile:"empty-array-only"),
Field(name:"categoryDeletionTombstones",type:"pairs:ID:Tombstone",optional:false,profile:"empty-array-only"),
Field(name:"labelDeletionTombstones",type:"pairs:ID:Tombstone",optional:false,profile:"empty-array-only"),
Field(name:"snapshotsByTraceID",type:"pairs:ID:TraceSnapshot",optional:false,profile:"empty-array-only"),
Field(name:"snapshotEventsByTraceID",type:"pairs:ID:[]TraceSnapshot",optional:false,profile:"empty-array-only"),
Field(name:"committedReceiptsByInteractionID",type:"pairs:uuid:Receipt",optional:false,profile:"empty-array-only"),
Field(name:"changeRecords",type:"[]ChangeRecord",optional:false,profile:"empty-array-only"),
],
]
func validateType(_ value:J,_ name:String) throws {
    if name == "TraceStatus" { try validateType(value,"enum:pending,completed,unfinished,deferred,changed,returnedToPool,cancelledDraft,abandoned");return }
    if name.hasPrefix("[]") {
        guard case .array(let a)=value else {throw OracleError.rejected("schema-invalid")}
        try demand(a.count<=16,"resource-limit");for v in a {try validateType(v,String(name.dropFirst(2)))};return
    }
    if name.hasPrefix("enum:") {try demand(name.dropFirst(5).split(separator:",").contains(Substring(value.string)));return}
    if name.hasPrefix("enumint:") {guard case .number(let n)=value else {throw OracleError.rejected("schema-invalid")};try demand(name.dropFirst(8).split(separator:",").contains(Substring(n)));return}
    switch name {
    case "string":guard case .string = value else {throw OracleError.rejected("schema-invalid")};return
    case "bool":guard case .bool = value else {throw OracleError.rejected("schema-invalid")};return
    case "int","uint","stamp":
        guard case .number(let n)=value else {throw OracleError.rejected("schema-invalid")}
        if name=="int" {try demand(Int64(n) != nil)} else {try demand(UInt64(n) != nil && !n.hasPrefix("-"))}
        if name=="stamp" {try demand(Double(bitPattern:UInt64(n)!).isFinite)};return
    case "uuid":try demand(value.string.count==36 && UUID(uuidString:value.string)?.uuidString==value.string);return
    default:break
    }
    guard let fs=fields[name],case .object(let object)=value else {throw OracleError.rejected("profile-violation")}
    try demand(Set(object.keys).isSubset(of:Set(fs.map{$0.name})))
    for f in fs {
        guard let v=object[f.name] else {try demand(f.optional);continue}
        if f.optional && f.name != "descriptionText" {throw OracleError.rejected("profile-violation")}
        if f.profile == "empty-array-only" {try demand(v == .array([]),"profile-violation");continue}
        try validateType(v,f.type)
        if ["active","pending"].contains(f.profile) {try demand(v == .string(f.profile),"profile-violation")}
        if ["0","1"].contains(f.profile) {try demand(v == .number(f.profile),"profile-violation")}
        if f.name == "plannedSubtasks" {try demand(v.array.count<=2,"resource-limit")}
        if f.name == "noteEntries" {try demand(v.array.count<=1,"resource-limit")}
    }
    if name=="LocalDate" {for v in object.values {guard case .number(let n)=v else{throw OracleError.rejected("schema-invalid")};try demand(Int32(n) != nil)}}
}
