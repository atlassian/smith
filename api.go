package smith

const (
	Smith  = "smith"
	Domain = "smith.atlassian.com"

	// See docs/design/managing-resources.md
	CrFieldPathAnnotation  = Domain + "/CrReadyWhenFieldPath"
	CrFieldValueAnnotation = Domain + "/CrReadyWhenFieldValue"
	CrdSupportEnabled      = Domain + "/SupportEnabled"

	EventAnnotationResourceName = Domain + "/ResourceName"
	EventAnnotationReason       = Domain + "/Reason"

	EventReasonResourceInProgress = "ResourceInProgress"
	EventReasonResourceReady      = "ResourceReady"
	EventReasonResourceError      = "ResourceError"
	EventReasonBundleInProgress   = "BundleInProgress"
	EventReasonBundleReady        = "BundleReady"
	EventReasonBundleError        = "BundleError"
	EventReasonUnknown            = "Unknown"
)
