package engine

type EventType string

const (
	EventManifestGenerated        EventType = "MANIFEST_GENERATED"
	EventManifestValidationFailed EventType = "MANIFEST_VALIDATION_FAILED"
	EventManifestDiffComputed     EventType = "MANIFEST_DIFF_COMPUTED"
	EventSchemaAltering           EventType = "SCHEMA_ALTERING"
	EventSchemaAltered            EventType = "SCHEMA_ALTERED"
	EventSchemaMigrationFailed    EventType = "SCHEMA_MIGRATION_FAILED"
	EventSnapshotCreated          EventType = "SNAPSHOT_CREATED"
	EventSnapshotRestored         EventType = "SNAPSHOT_RESTORED"
	EventDataSeeded               EventType = "DATA_SEEDED"
	EventRouteAdded               EventType = "ROUTE_ADDED"
	EventRouteUpdated             EventType = "ROUTE_UPDATED"
	EventRouteRemoved             EventType = "ROUTE_REMOVED"
	EventScriptValidationStarted  EventType = "SCRIPT_VALIDATION_STARTED"
	EventScriptValidationPassed   EventType = "SCRIPT_VALIDATION_PASSED"
	EventScriptValidationFailed   EventType = "SCRIPT_VALIDATION_FAILED"
	EventScriptLoaded             EventType = "SCRIPT_LOADED"
	EventScriptExecuted           EventType = "SCRIPT_EXECUTED"
	EventScriptError              EventType = "SCRIPT_ERROR"
	EventSessionStarted           EventType = "SESSION_STARTED"
	EventUserPromptReceived       EventType = "USER_PROMPT_RECEIVED"
	EventLLMRequestStarted        EventType = "LLM_REQUEST_STARTED"
	EventLLMRequestCompleted      EventType = "LLM_REQUEST_COMPLETED"
	EventHTTPRequestReceived      EventType = "HTTP_REQUEST_RECEIVED"
	EventHTTPResponseSent         EventType = "HTTP_RESPONSE_SENT"
	EventLogEmitted               EventType = "LOG_EMITTED"
)

type Event struct {
	Type EventType
	Data any
}
