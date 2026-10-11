# Decoding interpretations

Where gowemf's decoders accept input that the specifications describe
differently, and why. Reach for this when a real file decodes although a
strict reading would reject it, or before tightening a decoder. Coverage is in
[decoding coverage](../coverage/decoding.md).

## Embedded placeable-WMF compatibility

The EMF+ image at byte 928 of pinned `nested_wmf.emf` uses:

1. A placeable WMF header padded from 22 to 24 bytes.
2. A standard WMF stream following that header.
3. A MetafileDataSize that counts only the standard WMF stream.

This differs from the straightforward interpretation of MS-EMFPLUS §2.2.2.27.
gowemf accepts this specific representation only when the placeable signature,
standard WMF type/header-size, independently declared WMF word count, enclosing
object length and maximum three-byte final padding all agree. `AlignedPlaceable`
reports the interpretation explicitly. `MetafileBytes` removes exactly the two
alignment bytes; ordinary `Walk` remains strict about standard WMF framing.
There is a generated regression fixture; the third-party bytes are never tracked.

The normalized WMF also contains two enhanced-metafile escape fragments. Their
ByteCounts are 8,226 and 7,354: 34 bytes of metadata plus the **current fragment**
length, not 34 plus the entire embedded stream's length as the ByteCount prose in
MS-WMF §2.3.6.25 states. The typed envelope follows the bounded fragment layout.
The whole embedded-EMF checksum is not checked by a single-record decoder; the
sample's first checksum is `0x006f`, while the one's-complement XOR of the joined
EMF bytes is `0x4c6f`. The WMF fallback is independently framed and streamed;
no unverified embedded EMF is automatically selected or played.

## Driver-string layout

MS-EMFPLUS §2.3.4.6 gives an all-glyph position-array size formula, but says that
RealizedAdvance specifies only the first position. The decoder accepts either
unambiguously sized layout within the record, reports `StoredPositions`, and
retains only the first meaningful position when that option is set. Ignored
positions in a full array are not treated as active coordinates. Generated tests
cover both forms, unaligned glyph/float boundaries, optional matrices and ignored
non-finite positions. This interpretation has not yet been checked against Windows.
