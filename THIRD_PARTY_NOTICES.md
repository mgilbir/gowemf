# Third-party notices

## GIF format reference

GIF support uses Go's standard-library decoder. The GIF89a specification used
as the format reference requests the following acknowledgment:

The Graphics Interchange Format(c) is the Copyright property of CompuServe
Incorporated. GIF(sm) is a Service Mark property of CompuServe Incorporated.

Reference: https://www.w3.org/Graphics/GIF/spec-gif89a.txt

## TIFF format reference

The TIFF implementation is independently written from the TIFF Revision 6.0
specification (Aldus Corporation, June 3, 1992), consulted at
https://trap.mtview.ca.us/~tom/tech/file-formats/TIFF.html . No TIFF implementation
code or external binary image fixtures are included.

## Windows code page data

`codepage_tables.go` is generated from Microsoft's Windows best-fit code page
files (bestfit874.txt and bestfit1250.txt through bestfit1258.txt), which
MS-UCODEREF 2.2.2 names as the normative code page data, as published by the
Unicode Consortium at
https://www.unicode.org/Public/MAPPINGS/VENDORS/MICSFT/WindowsBestFit/ . Only
the byte-to-UTF-16 mappings are retained. The pinned files are downloaded to
`.external/codepages` and are not committed. They are distributed under the
Unicode License V3:

UNICODE LICENSE V3

COPYRIGHT AND PERMISSION NOTICE

Copyright © 1991-2026 Unicode, Inc.

NOTICE TO USER: Carefully read the following legal agreement. BY
DOWNLOADING, INSTALLING, COPYING OR OTHERWISE USING DATA FILES, AND/OR
SOFTWARE, YOU UNEQUIVOCALLY ACCEPT, AND AGREE TO BE BOUND BY, ALL OF THE
TERMS AND CONDITIONS OF THIS AGREEMENT. IF YOU DO NOT AGREE, DO NOT
DOWNLOAD, INSTALL, COPY, DISTRIBUTE OR USE THE DATA FILES OR SOFTWARE.

Permission is hereby granted, free of charge, to any person obtaining a
copy of data files and any associated documentation (the "Data Files") or
software and any associated documentation (the "Software") to deal in the
Data Files or Software without restriction, including without limitation
the rights to use, copy, modify, merge, publish, distribute, and/or sell
copies of the Data Files or Software, and to permit persons to whom the
Data Files or Software are furnished to do so, provided that either (a)
this copyright and permission notice appear with all copies of the Data
Files or Software, or (b) this copyright and permission notice appear in
associated Documentation.

THE DATA FILES AND SOFTWARE ARE PROVIDED "AS IS", WITHOUT WARRANTY OF ANY
KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT OF
THIRD PARTY RIGHTS.

IN NO EVENT SHALL THE COPYRIGHT HOLDER OR HOLDERS INCLUDED IN THIS NOTICE
BE LIABLE FOR ANY CLAIM, OR ANY SPECIAL INDIRECT OR CONSEQUENTIAL DAMAGES,
OR ANY DAMAGES WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS,
WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION,
ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THE DATA
FILES OR SOFTWARE.

Except as contained in this notice, the name of a copyright holder shall
not be used in advertising or otherwise to promote the sale, use or other
dealings in these Data Files or Software without prior written
authorization of the copyright holder.

## Unicode bidirectional data

`internal/bidi/tables.go` is generated from the Unicode Character Database
18.0.0 files `extracted/DerivedBidiClass.txt` and `BidiBrackets.txt`
(https://www.unicode.org/Public/18.0.0/ucd/); only the Bidi_Class of each
code point and the paired brackets are retained. The conformance tests
`BidiTest.txt` and `BidiCharacterTest.txt` are used under make test-external.
The pinned files are downloaded to `.external/unicode` and are not committed.
They are distributed under the Unicode License V3 reproduced in the Windows
code page data section above. The algorithm is implemented from Unicode
Standard Annex #9 revision 52.

## Test-only typesetting dependency

The separate `rendercheck` module, which holds render-oracle tests only, uses
`github.com/mgilbir/forme` v0.9.0 (MIT License, Copyright (c) 2026 Miguel
Eduardo Gil Biraud) to measure and outline text with the system's Liberation
Sans. It is not a dependency of the gowemf module and no forme source is
copied. Fonts are read from the system and never committed.

## Color-management dependency

gowemf uses `github.com/mgilbir/golittlecms` at revision
`f6af7cfe1556bc222c4572fcbc3ec0e1773b519d` for ICC color transforms. Its license
notice is reproduced below. No golittlecms implementation source has been copied
into this repository; the module is a dependency. Its GPL-licensed optional
Little-CMS plugins are not part of the dependency.

## golittlecms / Little-CMS

MIT License

Copyright (c) 2026 Miguel Eduardo Gil Biraud

This software is a Go port of Little-CMS (https://github.com/mm2/Little-CMS):
Copyright (c) 2023 Marti Maria Saguer

Permission is hereby granted, free of charge, to any person obtaining
a copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be
included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
