package corpus

// UnicodeURL hosts the Unicode Character Database release whose bidirectional
// data internal/bidigen turns into internal/bidi/tables.go, and whose
// conformance tests check internal/bidi. The files are distributed under the
// Unicode License V3; see THIRD_PARTY_NOTICES.md. They are downloaded into
// .external/unicode only to regenerate and verify the tables.
const UnicodeURL = "https://www.unicode.org/Public/18.0.0/ucd/"

// UnicodeFiles lists the pinned Unicode 18.0.0 files.
var UnicodeFiles = []File{
	{Path: "extracted/DerivedBidiClass.txt", SHA256: "d9e23222522551348ea1ccfbb4f62efbf98982afb95840f8959c08ed992c5607", Bytes: 176412},
	{Path: "BidiBrackets.txt", SHA256: "4b3b62e4a14b84ee752808c810c602534921c09a4a1bf78cfbee566d66c125b3", Bytes: 8992},
	{Path: "BidiTest.txt", SHA256: "9af2f882a4ab50912e388f069a673b94eacd82fa6d07d20a3ff7f3c759e905aa", Bytes: 7959988},
	{Path: "BidiCharacterTest.txt", SHA256: "045b24d2c8ab066951bd32fe8c6b4de34647f72b5b1c7df0265f24ab53573e01", Bytes: 6880771},
}
