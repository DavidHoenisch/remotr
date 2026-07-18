package audit

import "github.com/DavidHoenisch/remotr/internal/executor"

func PublicDetail(path, value string) executor.SafeField {
	return executor.SafeField{Path: path, Sensitivity: executor.SafePublic, Projection: executor.SafeValue, Text: value}
}

func MetadataDetail(path, value string) executor.SafeField {
	return executor.SafeField{Path: path, Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeMetadata, Text: value}
}

func FingerprintDetail(path, value string) executor.SafeField {
	return executor.SafeField{Path: path, Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeFingerprint, Text: value}
}

func SecretReferenceDetail(path, value string) executor.SafeField {
	return executor.SafeField{Path: path, Sensitivity: executor.SafeSecret, Projection: executor.SafeReference, Text: value}
}

func PresenceDetail(path string, present bool) executor.SafeField {
	return executor.SafeField{Path: path, Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafePresence, Present: &present}
}

func CountDetail(path string, count int) executor.SafeField {
	return executor.SafeField{Path: path, Sensitivity: executor.SafeSensitiveMetadata, Projection: executor.SafeCount, Count: &count}
}
