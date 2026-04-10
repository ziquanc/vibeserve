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

	// Planning events
	EventPlanCreated  EventType = "PLAN_CREATED"  // Data: PlanInfo
	EventStepStarted  EventType = "STEP_STARTED"  // Data: StepInfo
	EventStepCompleted EventType = "STEP_COMPLETED" // Data: StepInfo

	EventBlueprintProposed EventType = "BLUEPRINT_PROPOSED"
	EventBlueprintRefined  EventType = "BLUEPRINT_REFINED"
	EventBlueprintApproved EventType = "BLUEPRINT_APPROVED"

	// Auto-fix events
	EventAutoFixStarted   EventType = "AUTO_FIX_STARTED"   // Data: AutoFixInfo
	EventAutoFixCompleted EventType = "AUTO_FIX_COMPLETED" // Data: AutoFixInfo
)

// PlanInfo describes the execution plan.
type PlanInfo struct {
	Steps []string
	Total int
}

// StepInfo describes a single step's progress.
type StepInfo struct {
	Index   int
	Total   int
	Description string
	Changes []string // summary of what changed in this step
}

type Event struct {
	Type EventType
	Data any
}

// AutoFixInfo describes an auto-fix attempt.
type AutoFixInfo struct {
	Attempt   int      // which attempt (1-based)
	Max       int      // max attempts
	Errors    []string // compilation errors found
	Fixed     bool     // whether the fix succeeded
	StepIndex int      // -1 if not step-level (direct apply)
	StepDesc  string   // step description (if step-level)
}
