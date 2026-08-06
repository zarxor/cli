package adapters

// ownershipStatus describes how confidently an adapter can associate the
// active executable with the package manager it would use for an update.
// Unknown keeps compatibility with hosts where the package-manager probe is
// unavailable; a confirmed mismatch is always handled conservatively.
type ownershipStatus uint8

const (
	ownershipUnknown ownershipStatus = iota
	ownershipOwned
	ownershipNotOwned
)
