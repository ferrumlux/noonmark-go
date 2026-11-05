// SPDX-License-Identifier: AGPL-3.0-only
// Schema derived from Noonmark's v7 Codable declarations. No Swift oracle yet.
package v7

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/ferrumlux/noonmark/ports/mygo/domain-codec/canonical"
	"github.com/ferrumlux/noonmark/ports/mygo/domain/core"
)

var (
	ErrMalformed                = errors.New("malformed v7 package")
	ErrUnsupportedVersion       = errors.New("unsupported data package version")
	ErrDomainValidationRequired = errors.New("complete classification audit validator is required")
	ErrMissingReference         = errors.New("missing v7 reference")
	ErrDuplicate                = errors.New("duplicate v7 identity")
	ErrWriteVerification        = errors.New("persisted package verification failed")
)

var ErrResourceLimit = errors.New("v7 domain validation work limit exceeded")

const MaximumRevisionsPerSeries = 2048
const MaximumTotalRevisions = 4096

type fields map[string]string

var objects = map[string]fields{
	"Envelope":               {"formatVersion": "int", "snapshot": "Snapshot"},
	"Snapshot":               {"days": "[]Day", "taskCycleSeries": "[]Series", "chains": "[]Chain", "definitions": "[]Definition", "traces": "[]Trace", "subtasks": "[]Subtask", "preferences": "Preferences", "classifications": "Classifications", "ideas": "[]Idea"},
	"ID":                     {"rawValue": "uuid"},
	"LocalDate":              {"year": "int", "month": "int", "day": "int"},
	"Day":                    {"id": "ID", "date": "LocalDate", "lockedAt?": "stamp", "reviewSummary?": "string", "reviewUnfinishedReason?": "string", "reviewTomorrowNote?": "string", "createdAt": "stamp", "updatedAt": "stamp"},
	"Chain":                  {"id": "ID", "state": "enum:active,abandoned", "cycleMembership?": "Membership", "noteEntries": "[]Note", "createdAt": "stamp", "updatedAt": "stamp"},
	"Membership":             {"seriesID": "ID", "occurrenceDate": "LocalDate"},
	"Note":                   {"id": "ID", "body": "string", "createdAt": "stamp", "updatedAt": "stamp", "deletedAt?": "stamp"},
	"PlannedSubtask":         {"id": "ID", "lineageID": "ID", "title": "string", "difficulty": "enumint:1,2,3", "position": "int", "createdAt": "stamp"},
	"Definition":             {"id": "ID", "chainID": "ID", "sequence": "int", "title": "string", "descriptionText?": "string", "plannedSubtasks": "[]PlannedSubtask", "createdAt": "stamp", "contentUpdatedAt": "stamp", "supersededAt?": "stamp", "supersededByDefinitionID?": "ID"},
	"Trace":                  {"id": "ID", "chainID": "ID", "definitionID": "ID", "date": "LocalDate", "status": "TraceStatus", "priority": "int", "pinOrder?": "int", "continuationSeq": "int", "descriptionText?": "string", "noteEntries": "[]Note", "manualProgressPercent?": "int", "carriedFromTraceID?": "ID", "changedToTraceID?": "ID", "createdAt": "stamp", "contentUpdatedAt": "stamp", "completedAt?": "stamp", "settledAt?": "stamp", "draftCancellationID?": "uuid", "draftCancelledOn?": "LocalDate"},
	"Subtask":                {"id": "ID", "lineageID": "ID", "traceID": "ID", "title": "string", "status": "enum:pending,completed,unfinished,deferred,abandoned,cancelledDraft", "difficulty": "enumint:1,2,3", "position": "int", "carriedFromSubtaskID?": "ID", "createdAt": "stamp", "updatedAt": "stamp", "completedAt?": "stamp", "settledAt?": "stamp", "draftCancellationID?": "uuid"},
	"Idea":                   {"id": "ID", "body": "string", "categoryID?": "ID", "labelIDs": "[]ID", "createdAt": "stamp", "updatedAt": "stamp", "deletedAt?": "stamp", "pinnedAt?": "stamp"},
	"Preferences":            {"theme": "enum:coolGray,warmPaper", "language": "enum:chinese,english", "themeLanguageUpdatedAt": "stamp", "themeLanguageWriterID": "string", "dataMode": "enum:localFirst,onlineFirst", "backupPolicy": "BackupPolicy", "localFirstSyncPolicy": "SyncPolicy", "settingsPoemDisplayPolicy": "PoemPolicy", "syncEndpointOptions": "[]SyncEndpoint"},
	"BackupPolicy":           {"frequency": "enum:off,daily,weekly", "destination": "enum:iCloudDrive,s3"},
	"SyncPolicy":             {"enabled": "bool", "endpoint": "enum:iCloud,s3,webDAV,localFolder", "mode": "enum:manual,automatic", "intervalSeconds": "int", "generateConflictCopy": "bool", "snapshotRetention": "RetentionPolicy"},
	"RetentionPolicy":        {"indexRetentionDays": "int", "retentionIndexesDaily": "int"},
	"PoemPolicy":             {"enabled": "bool", "text": "string"},
	"SyncEndpoint":           {"kind": "enum:customEndpoint,iCloud,s3,webDAV,localFolder", "title": "string", "description": "string", "availability": "enum:available,planned"},
	"Series":                 {"id": "ID", "title": "string", "descriptionText?": "string", "plannedSubtasks": "[]PlannedSubtask", "categoryID?": "ID", "labelIDs": "[]ID", "startDate": "LocalDate", "endDate": "LocalDate", "schedule": "Schedule", "endCondition": "EndCondition", "planRevisions": "[]PlanRevision", "classificationRevisions": "[]ClassificationRevision", "cancellationFacts": "[]CancellationFact", "createdAt": "stamp", "updatedAt": "stamp"},
	"PlanRevision":           {"id": "uuid", "effectiveFrom": "LocalDate", "endConditionAnchorDate": "LocalDate", "schedule": "Schedule", "endCondition": "EndCondition", "recordedAt": "stamp"},
	"ClassificationRevision": {"id": "uuid", "previousRevisionID?": "uuid", "interactionID": "uuid", "source": "Source", "decisionID?": "uuid", "categoryID?": "ID", "labelIDs": "[]ID", "recordedAt": "stamp"},
	"CancellationFact":       {"id": "uuid", "scope": "CancellationScope", "reason": "enum:userSkipped,planRevision,stoppedEarly,completionTargetReached", "recordedAt": "stamp"},
	"Classifications":        {"formatVersion": "int", "revision": "uint", "categories": "pairs:ID:Category", "labels": "pairs:ID:Label", "currentByChainID": "pairs:ID:Current", "relationHistory": "[]RelationHistory", "categoryMerges": "pairs:ID:Merge", "labelMerges": "pairs:ID:Merge", "categoryDeletionTombstones": "pairs:ID:Tombstone", "labelDeletionTombstones": "pairs:ID:Tombstone", "snapshotsByTraceID": "pairs:ID:TraceSnapshot", "snapshotEventsByTraceID": "pairs:ID:[]TraceSnapshot", "committedReceiptsByInteractionID": "pairs:uuid:Receipt", "changeRecords": "[]ChangeRecord"},
	"Category":               {"id": "ID", "name": "string", "canonicalKey": "string", "canonicalKeyVersion": "string", "colorHex": "string", "presentationApproval": "Approval", "lifecycle": "Lifecycle", "nameVersions": "[]NameVersion", "createdAt": "stamp", "updatedAt": "stamp"},
	"Label":                  {"id": "ID", "name": "string", "canonicalKey": "string", "canonicalKeyVersion": "string", "colorHex": "string", "lifecycle": "Lifecycle", "nameVersions": "[]NameVersion", "createdAt": "stamp", "updatedAt": "stamp"},
	"NameVersion":            {"id": "uuid", "name": "string", "canonicalKey": "string", "canonicalKeyVersion": "string", "validFrom": "stamp", "validUntil?": "stamp"},
	"CategoryRelation":       {"categoryID": "ID", "source": "Source", "decisionID?": "uuid", "createdAt": "stamp", "updatedAt": "stamp", "revision": "uint"},
	"LabelRelation":          {"labelID": "ID", "source": "Source", "decisionID?": "uuid", "createdAt": "stamp", "updatedAt": "stamp", "revision": "uint"},
	"Current":                {"category?": "CategoryRelation", "labels": "[]LabelRelation"},
	"RelationHistory":        {"id": "uuid", "kind": "ItemKind", "chainID": "ID", "itemID": "string", "originSource": "Source", "originDecisionID?": "uuid", "createdAt": "stamp", "createdRevision": "uint", "removedBySource": "Source", "removedByDecisionID?": "uuid", "removedAt": "stamp", "removedRevision": "uint"},
	"Merge":                  {"id": "uuid", "sourceID": "ID", "targetID": "ID", "mergedAt": "stamp", "revision": "uint", "changeRecordID": "uuid"},
	"Tombstone":              {"id": "uuid", "itemID": "ID", "deletedAt": "stamp", "revision": "uint", "changeRecordID": "uuid"},
	"HistoricalCategory":     {"id": "ID", "name": "string", "colorHex": "string", "presentationApproval": "Approval"},
	"HistoricalLabel":        {"id": "ID", "name": "string", "colorHex": "string"},
	"TraceSnapshot":          {"id": "uuid", "traceID": "ID", "status": "TraceStatus", "category?": "HistoricalCategory", "labels": "[]HistoricalLabel", "capturedAt": "stamp", "revision": "uint"},
	"Receipt":                {"planID": "uuid", "revision": "uint", "notices": "[]Notice", "changeRecordID": "uuid", "decisionID?": "uuid", "changeRecordIntegrityDigest": "string"},
	"ChangeRecord":           {"id": "uuid", "planID": "uuid", "interactionID": "uuid", "source": "Source", "decisionID?": "uuid", "changes": "[]PlanChange", "notices": "[]Notice", "committedAt": "stamp", "revision": "uint", "planDigest": "string", "integrityDigest": "string"},
	"PlanItem":               {"id?": "string", "kind": "ItemKind", "name": "string", "colorHex": "string", "resolution": "enum:existing,historicalAlias,new"},
	"SelectionPreview":       {"category?": "PlanItem", "labels": "[]PlanItem"},
	"MergeImpact":            {"currentChainIDs": "[]ID", "historicalTraceIDs": "[]ID", "historicalEventIDs": "[]uuid", "migratedRelationCount": "int", "deduplicatedRelationCount": "int"},
	"ReferenceSummary":       {"currentRelationCount": "int", "relationHistoryCount": "int", "historicalEventCount": "int", "mergeSourceCount": "int", "mergeTargetCount": "int"},
}
var aliases = map[string]string{
	"TraceStatus": "enum:pending,completed,unfinished,deferred,changed,returnedToPool,cancelledDraft,abandoned",
	"Approval":    "enum:userApproved,pendingAIReview", "Lifecycle": "enum:active,archived,merged", "ItemKind": "enum:category,label",
}
var unions = map[string]map[string]fields{
	"Schedule":          {"everyDays": {"_0": "int"}, "selectedWeekdays": {"weekdays": "[]enumint:1,2,3,4,5,6,7", "everyWeeks": "int"}},
	"EndCondition":      {"durationDays": {"_0": "int"}, "onDate": {"_0": "LocalDate"}, "completionTarget": {"count": "int", "latestDate": "LocalDate"}},
	"CancellationScope": {"occurrence": {"_0": "LocalDate"}, "followingDates": {"after": "LocalDate"}},
	"Source":            {"userDirect": {}, "zhulongSuggestion": {"sessionID": "uuid", "draftID": "uuid", "draftVersion": "int", "evidenceID": "uuid"}, "automaticAI": {"jobID": "uuid", "generation": "int"}, "inherited": {"fromChainID": "ID"}, "deterministicDomainAction": {"reason": "string"}},
	"Notice":            {"duplicateLabelCollapsed": {"name": "string"}, "existingItemReused": {"kind": "ItemKind", "inputName": "string", "currentName": "string", "matchedHistoricalAlias": "bool"}},
	"PlanChange":        {"create": {"kind": "ItemKind", "itemID": "string", "name": "string", "colorHex": "string"}, "merge": {"kind": "ItemKind", "sourceID": "string", "sourceName": "string", "targetID": "string", "targetName": "string", "sourceLifecycle": "Lifecycle", "impact": "MergeImpact"}, "hardDelete": {"kind": "ItemKind", "itemID": "string", "name": "string", "lifecycle": "Lifecycle", "verifiedReferences": "ReferenceSummary"}, "setCurrent": {"chainID": "ID", "before": "SelectionPreview", "after": "SelectionPreview"}, "rename": {"kind": "ItemKind", "itemID": "string", "beforeName": "string", "afterName": "string"}, "categoryPresentationApproval": {"itemID": "string", "name": "string", "before": "Approval", "after": "Approval"}, "lifecycle": {"kind": "ItemKind", "itemID": "string", "name": "string", "before": "Lifecycle", "after": "Lifecycle"}},
}

func malformed(path, reason string) error {
	return fmt.Errorf("%w at %s: %s", ErrMalformed, path, reason)
}
func validateFields(v canonical.Value, fs fields, path string) error {
	if v.Kind != canonical.Object {
		return malformed(path, "expected object")
	}
	allowed := map[string]bool{}
	for key, typ := range fs {
		optional := strings.HasSuffix(key, "?")
		key = strings.TrimSuffix(key, "?")
		allowed[key] = true
		child, ok := v.Object[key]
		if !ok {
			if optional {
				continue
			}
			return malformed(path+"."+key, "required field missing")
		}
		if err := validateType(child, typ, path+"."+key); err != nil {
			return err
		}
	}
	for key := range v.Object {
		if !allowed[key] {
			return malformed(path+"."+key, "unknown field")
		}
	}
	return nil
}
func validateType(v canonical.Value, typ, path string) error {
	if alias, ok := aliases[typ]; ok {
		return validateType(v, alias, path)
	}
	if strings.HasPrefix(typ, "[]") {
		if v.Kind != canonical.Array {
			return malformed(path, "expected array")
		}
		for i, c := range v.Array {
			if err := validateType(c, typ[2:], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	}
	if strings.HasPrefix(typ, "pairs:") {
		parts := strings.SplitN(strings.TrimPrefix(typ, "pairs:"), ":", 2)
		if v.Kind != canonical.Array || len(v.Array)%2 != 0 {
			return malformed(path, "expected alternating dictionary array")
		}
		seen := map[string]bool{}
		for i := 0; i < len(v.Array); i += 2 {
			if err := validateType(v.Array[i], parts[0], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
			key := strings.ToUpper(v.Array[i].String)
			if parts[0] == "ID" {
				key = strings.ToUpper(v.Array[i].Object["rawValue"].String)
			}
			if seen[key] {
				return fmt.Errorf("%w at %s: dictionary key", ErrDuplicate, path)
			}
			seen[key] = true
			if err := validateType(v.Array[i+1], parts[1], fmt.Sprintf("%s[%d]", path, i+1)); err != nil {
				return err
			}
		}
		return nil
	}
	if fs, ok := objects[typ]; ok {
		if err := validateFields(v, fs, path); err != nil {
			return err
		}
		if typ == "LocalDate" && !localDate(v).Valid() {
			return malformed(path, "invalid civil date")
		}
		return nil
	}
	if cases, ok := unions[typ]; ok {
		if v.Kind != canonical.Object || len(v.Object) != 1 {
			return malformed(path, "expected exactly one enum case")
		}
		for key, child := range v.Object {
			fs, ok := cases[key]
			if !ok {
				return malformed(path, "unknown enum case")
			}
			return validateFields(child, fs, path+"."+key)
		}
	}
	if strings.HasPrefix(typ, "enum:") {
		if v.Kind == canonical.String {
			for _, x := range strings.Split(typ[5:], ",") {
				if x == v.String {
					return nil
				}
			}
		}
		return malformed(path, "unknown enum string")
	}
	if strings.HasPrefix(typ, "enumint:") {
		if err := validateType(v, "int", path); err != nil {
			return err
		}
		for _, x := range strings.Split(typ[8:], ",") {
			if x == v.TextNumber {
				return nil
			}
		}
		return malformed(path, "unknown enum integer")
	}
	switch typ {
	case "string":
		if v.Kind == canonical.String {
			return nil
		}
	case "bool":
		if v.Kind == canonical.Boolean {
			return nil
		}
	case "uuid":
		if v.Kind == canonical.String && validUUID(v.String) {
			return nil
		}
	case "int":
		if v.Kind == canonical.Number {
			if _, err := strconv.ParseInt(v.TextNumber, 10, 64); err == nil {
				return nil
			}
		}
	case "uint":
		if v.Kind == canonical.Number {
			if _, err := strconv.ParseUint(v.TextNumber, 10, 64); err == nil {
				return nil
			}
		}
	case "stamp":
		if v.Kind == canonical.Number {
			if n, err := strconv.ParseUint(v.TextNumber, 10, 64); err == nil && !math.IsNaN(math.Float64frombits(n)) && !math.IsInf(math.Float64frombits(n), 0) {
				return nil
			}
		}
	}
	return malformed(path, "invalid "+typ)
}
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func signed(v canonical.Value) int64    { n, _ := strconv.ParseInt(v.TextNumber, 10, 64); return n }
func unsigned(v canonical.Value) uint64 { n, _ := strconv.ParseUint(v.TextNumber, 10, 64); return n }
func localDate(v canonical.Value) core.LocalDate {
	return core.LocalDate{Year: int(signed(v.Object["year"])), Month: int(signed(v.Object["month"])), Day: int(signed(v.Object["day"]))}
}
