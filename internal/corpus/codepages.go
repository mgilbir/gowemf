package corpus

// CodePageURL hosts the Windows best-fit code page data files that MS-UCODEREF
// 2.2.2 names as the normative codepage data. The files are distributed under
// the Unicode License V3; see THIRD_PARTY_NOTICES.md. They are downloaded into
// .external/codepages only to regenerate and verify codepage_tables.go.
const CodePageURL = "https://www.unicode.org/Public/MAPPINGS/VENDORS/MICSFT/WindowsBestFit/"

// CodePages lists the single-byte ANSI code pages used by text playback.
var CodePages = []File{
	{Path: "bestfit874.txt", SHA256: "663f43ca662e037c4534cb16298b560f29ce29c27b49b3589601ec3d97dd89fd", Bytes: 19653},
	{Path: "bestfit1250.txt", SHA256: "cef9f171e67b09445bcb3f9ffccdc89418250ff825f1bd2d29a92d2074d7a53b", Bytes: 36656},
	{Path: "bestfit1251.txt", SHA256: "59ec85612ff908d9da0e877893c935941e56b13a2882b4fb9c9599be3d1ce4e7", Bytes: 35779},
	{Path: "bestfit1252.txt", SHA256: "72ea23c939c5b26fae7aded0207b327e2f3902d7d3c168d7087f5cfc38ee76a9", Bytes: 37057},
	{Path: "bestfit1253.txt", SHA256: "ea80c442aff7f09b36da6335f85f8e527f51c146beeb9825ec00d1b6ca99a99e", Bytes: 34013},
	{Path: "bestfit1254.txt", SHA256: "3d02512087634dc493b720992b590277736ffb2d5b0b665d69b6b9727e2c361a", Bytes: 36451},
	{Path: "bestfit1255.txt", SHA256: "fdd4bdda74f6571d89171b0070ac052cd3714c395dc3d1799bcd5e4a4da6f83a", Bytes: 20822},
	{Path: "bestfit1256.txt", SHA256: "745c447ada04a838da8bea406c13f446c7453b6371e8c6c7863a632443d56007", Bytes: 29404},
	{Path: "bestfit1257.txt", SHA256: "b8c5d7f3b8c25c3d5625d44dd3d6ee7a06e652ddf77373d050282c1cb7517366", Bytes: 16235},
	{Path: "bestfit1258.txt", SHA256: "5d52a9357b7d6b5b5014ed5a51be0ff9809b0c33625793d2a4feaf502e0682f1", Bytes: 22252},
}
