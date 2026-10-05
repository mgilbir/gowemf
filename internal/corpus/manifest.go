// Package corpus describes opt-in third-party test inputs. The manifest records
// provenance, not a grant of redistribution rights for the underlying documents.
package corpus

const Revision = "732120980140d5ed64b482c470e0b625cdb1ab15"
const BaseURL = "https://raw.githubusercontent.com/apache/poi/" + Revision + "/test-data/"

type File struct {
	Path                      string
	SHA256                    string
	Bytes                     int64
	OuterRecords, PlusRecords int
}

// Counts were independently checked by a separate binary-framing inspection.
// They are not semantic or rendering oracle results.
var Files = []File{
	{"slideshow/santa.wmf", "fdc312bc0dbea47df16d3e3e98e2777e4aa537d225a703e3cde0177df56a592a", 28696, 582, 0},
	{"slideshow/wrench.emf", "3c9e27e68d0322daaff3477a957d46ae5b32b23f058d29e65d3947b6eeafb2cb", 6184, 153, 0},
	{"spreadsheet/SimpleEMF_windows.emf", "cec8d939fbb3d327104bcadf8886d1b31be999d01f61eef73ab012ba03d5c6c9", 27864, 31, 9},
	{"spreadsheet/SimpleEMF_mac.emf", "a4ad531ab19f4f085dd987ce18dc5e04ded3493190b99e43b6295ec31035bcbf", 133320, 2852, 0},
	{"document/vector_image.emf", "eaa202a7682e9f8db12bf4d6e76e0e1c6c99fbeb9a59577b96b482e1f46a8475", 7348, 279, 0},
	{"slideshow/nested_wmf.emf", "4dd0ea622b0feea6daff6b953bd0fe30cc3deb8dc561828f069f83d5bd5cef75", 40276, 588, 18},
}
