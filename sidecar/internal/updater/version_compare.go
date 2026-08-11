package updater

// IsStrictlyNewerSidecarVersion compares canonical `v<MAJOR>.<LETTERS>`
// versions without authorizing downgrades or malformed targets.
//
// Higher majors win. Within one major, longer letter tokens win before
// lexicographic order, so AA follows Z. A development build may accept any
// valid advertised version. Invalid input always fails closed.
func IsStrictlyNewerSidecarVersion(current, advertised string) bool {
	if current == "dev" {
		_, ok := parseSidecarVersion(advertised)
		return ok
	}
	cur, ok1 := parseSidecarVersion(current)
	adv, ok2 := parseSidecarVersion(advertised)
	if !ok1 || !ok2 {
		return false
	}
	if adv.major != cur.major {
		return adv.major > cur.major
	}
	if len(adv.letters) != len(cur.letters) {
		return len(adv.letters) > len(cur.letters)
	}
	return adv.letters > cur.letters
}
