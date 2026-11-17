package tfutil

import "regexp"

// UUIDPattern matches a UUID in its canonical textual form, the format of every ID the Reco API issues.
var UUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
