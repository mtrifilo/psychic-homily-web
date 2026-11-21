package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// planBinding ties a confirmed run to the dry run the owner reviewed.
type planBinding struct {
	// target names the database: ENVIRONMENT and host/dbname. It is part of
	// the digest, so a plan reviewed against one database cannot authorize
	// writes to another that happens to share ids.
	target string
	// expect is the digest the reviewed dry run printed; --confirm requires it.
	expect string
}

// rowFingerprint is what the digest binds for one row: its line, action and
// verdict, and for a row that would apply, its footprint: the artist's id and
// name, each show whose bill it rewrites with the bill's artist ids in order
// before the change, a merge's kept artist, and a split's acts with the
// existing artist each links or "new". Any change to these between the dry run
// and the confirmed run changes the digest.
func rowFingerprint(p *rowPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "line=%d|action=%s|verdict=%s", p.Row.Line, p.Row.Action, p.Status)
	if p.Status != statusWouldApply {
		return b.String()
	}
	fmt.Fprintf(&b, "|artist=%d:%s", p.Record.ArtistID, p.Record.ArtistName)
	if p.Row.Action == actionMerge {
		fmt.Fprintf(&b, "|kept=%d", p.TargetID)
	}
	for _, sh := range p.Record.Shows {
		ids := make([]string, len(sh.BillBefore))
		for i, a := range sh.BillBefore {
			ids[i] = fmt.Sprintf("%d", a.ArtistID)
		}
		fmt.Fprintf(&b, "|show=%d:[%s]", sh.ShowID, strings.Join(ids, ","))
	}
	if p.Row.Action == actionSplit {
		linked := map[string]uint{}
		for _, a := range p.Record.ActsLinked {
			linked[strings.ToLower(a.Name)] = a.ID
		}
		acts, _ := splitActNames(p.Row.Target, "")
		for _, name := range acts {
			if id, ok := linked[strings.ToLower(name)]; ok {
				fmt.Fprintf(&b, "|act=%s=%d", name, id)
			} else {
				fmt.Fprintf(&b, "|act=%s=new", name)
			}
		}
	}
	return b.String()
}

// shortHash is the first 8 bytes of a SHA-256, in hex.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// fingerprintPlans stamps each row with the hash of its fingerprint and
// returns the plan's digest: the hash of the target and every row's
// fingerprint, in file order.
func fingerprintPlans(target string, plans []*rowPlan) string {
	parts := make([]string, 0, len(plans)+1)
	parts = append(parts, target)
	for _, p := range plans {
		f := rowFingerprint(p)
		p.Fingerprint = shortHash(f)
		parts = append(parts, f)
	}
	return shortHash(strings.Join(parts, "\n"))
}
