package k8s

const (
	AnnotationPrefix = "bottlerocket-updater.aws/"

	AnnotationCurrentVersion = AnnotationPrefix + "current-version"
	AnnotationVariantID      = AnnotationPrefix + "variant-id"
	AnnotationArch           = AnnotationPrefix + "arch"
	AnnotationTargetVersion  = AnnotationPrefix + "target-version"
	AnnotationState          = AnnotationPrefix + "state"
	AnnotationRequestedAt    = AnnotationPrefix + "requested-at"
	AnnotationStartedAt      = AnnotationPrefix + "started-at"
	AnnotationCompletedAt    = AnnotationPrefix + "completed-at"
	AnnotationLastError      = AnnotationPrefix + "last-error"

	StateRequested = "requested"
	StateCordoning = "cordoning"
	StateDraining  = "draining"
	StateUpdating  = "updating"
	StateRebooting = "rebooting"
	StateCompleted = "completed"
	StateFailed    = "failed"

	StateKeyTargetVersion = "targetVersion"
	StateKeyFetchedDate   = "fetchedDate"
	StateKeyFetchedAt     = "fetchedAt"
	StateKeyParameterName = "parameterName"
)
