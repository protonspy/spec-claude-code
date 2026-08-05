package artifact

import (
	"strings"
	"testing"
)

const unsealed = "---\nautonomy: auto\nci: wait\n---\n\n# Sweep\n\nWhat this is.\n\n## Tasks\n\n- [ ] 1.1 (Unit) Do it\n"

// The canonicalization, frozen. Two properties, and both are load-bearing.
//
// The seal covers the file *minus its own checksum line* — a hash that included the
// line it is written on could never be satisfied. And it covers the LF-normalized
// text, so the same plan checked out with CRLF on Windows seals identically to the LF
// copy on Linux; without that, half the CI matrix would report permanent drift on a
// file nobody touched.
func TestSealCanonicalizationIsFrozen(t *testing.T) {
	// Golden, so a change to the canonicalization is a decision rather than an
	// accident: it would make every sealed plan in every workspace report drift.
	const golden = "335453e2799c7f47f40d84b3afc9b76f4c3af3181940d7f0f5bb907cbdfe5043"
	sum := Seal(unsealed)
	if sum != golden {
		t.Fatalf("the seal of a known plan changed: %s, was %s — every sealed plan now drifts", sum, golden)
	}

	if crlf := Seal(strings.ReplaceAll(unsealed, "\n", "\r\n")); crlf != sum {
		t.Errorf("CRLF sealed to %s and LF to %s — Windows would report permanent drift", crlf, sum)
	}

	sealed := Approve(unsealed)
	if !strings.Contains(sealed, "status: approved") {
		t.Fatalf("Approve did not write the status:\n%s", sealed)
	}
	if !strings.Contains(sealed, "checksum: "+Seal(sealed)) {
		t.Errorf("the written checksum does not describe the file it is in:\n%s", sealed)
	}
	if Seal(sealed) != Seal(strings.Replace(sealed, "checksum: "+Seal(sealed), "checksum: deadbeef", 1)) {
		t.Error("the checksum line is inside its own hash; no value could ever satisfy it")
	}
}

func TestApproveIsIdempotentAndResealFollowsTheContent(t *testing.T) {
	once := Approve(unsealed)
	if twice := Approve(once); twice != once {
		t.Errorf("approving twice changed the file:\n%s\n---\n%s", once, twice)
	}

	edited := strings.Replace(once, "- [ ] 1.1", "- [x] 1.1", 1)
	if Seal(edited) == Seal(once) {
		t.Fatal("an edit that changed a checkbox did not change the seal")
	}
	resealed := Reseal(edited)
	if !strings.Contains(resealed, "checksum: "+Seal(resealed)) {
		t.Error("Reseal did not record the content it was given")
	}
	if !strings.Contains(resealed, "status: approved") {
		t.Error("Reseal changed the status; it only recomputes the hash")
	}
}

// A file with no frontmatter still has to be sealable — `plan new` writes one, but a
// plan somebody wrote by hand may not have.
func TestApproveAddsAFrontmatterBlockWhenThereIsNone(t *testing.T) {
	out := Approve("# Sweep\n\nWhat this is.\n")
	if !strings.HasPrefix(out, "---\n") {
		t.Fatalf("no frontmatter block was added:\n%s", out)
	}
	if !strings.Contains(out, "status: approved") || !strings.Contains(out, "checksum: ") {
		t.Errorf("the seal was not written:\n%s", out)
	}
	if !strings.Contains(out, "# Sweep") {
		t.Error("the body did not survive")
	}
}
