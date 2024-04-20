# PACKAGE `index`

The **index** packge is responsible to convert any [`stage`](../../stage/README.md) into an ImageIndex.
If the _Stage_ is Single Architecture then an Image is built with a single Appropriate Platform and is appended to an empty ImageIndex.

The ImageIndex is stored in memory only, inorder to export the ImageIndex to the available formats take a look at [`export`](../../exports/README.md) package.