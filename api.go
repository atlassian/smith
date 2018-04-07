package smith

const (
	Domain = "smith.atlassian.com"

	// See docs/design/managing-resources.md
	CrFieldPathAnnotation  = Domain + "/CrReadyWhenFieldPath"
	CrFieldValueAnnotation = Domain + "/CrReadyWhenFieldValue"
	CrdSupportEnabled      = Domain + "/SupportEnabled"

	BundleNameLabel = Domain + "/BundleName"

	ReferenceModifierBindSecret = "bindsecret"
)
