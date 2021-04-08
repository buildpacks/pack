package fakes

//func NewFakeAssetLayer(layerContents string, assets ...dist.Asset) asset.AssetLayer {
//	ts := archive.NormalizedDateTime
//	tarBuilder := archive.TarBuilder{}
//	// TODO -Dan- do we need to add /cnb, /cnb/assets dirs?
//	for _, asset := range assets {
//		tarBuilder.AddFile(fmt.Sprintf("/cnb/assets/%s", asset.Sha256), 0644, ts, []byte(layerContents))
//	}
//	return asset.AssetLayer{
//		ReadCloser: tarBuilder.Reader(archive.DefaultTarWriterFactory()),
//		Assets:     assets,
//	}
//}
