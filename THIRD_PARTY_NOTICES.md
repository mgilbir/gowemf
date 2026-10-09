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
