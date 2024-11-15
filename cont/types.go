package cont

type ContentType string

const (
	AnyContenType ContentType = "*/*"

	ApplicationJson           ContentType = "application/json"
	ApplicationYaml           ContentType = "application/x-yaml"
	ApplicationPdf            ContentType = "application/pdf"
	ApplicationXml            ContentType = "application/xml"
	ApplicationZip            ContentType = "application/zip"
	ApplicationOgg            ContentType = "application/ogg"
	ApplicationFormUrlEncoded ContentType = "application/x-www-form-urlencoded"

	AudioMpeg      ContentType = "audio/mpeg"
	AudioXMsWma    ContentType = "audio/x-ms-wma"
	AudioRealAudio ContentType = "audio/vnd.rn-realaudio"
	AudioXWav      ContentType = "audio/x-wav"

	ImageGif    ContentType = "image/gif"
	ImageJpeg   ContentType = "image/jpeg"
	ImagePng    ContentType = "image/png"
	ImageTiff   ContentType = "image/tiff"
	ImageXIcon  ContentType = "image/x-icon"
	ImageSvgXml ContentType = "image/svg+xml"

	TextCss        ContentType = "text/css"
	TextCsv        ContentType = "text/csv"
	TextHtml       ContentType = "text/html"
	TextJavaScript ContentType = "text/javascript"
	TextPlain      ContentType = "text/plain"
	TextXml        ContentType = "text/xml"

	// video/mpeg
	// video/mp4
	// video/quicktime
	// video/x-ms-wmv
	// video/x-msvideo
	// video/x-flv
	// video/webm
)
