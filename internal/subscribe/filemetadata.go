package subscribe

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

var fileSize = regexp.MustCompile(`(?i)\b[0-9]+(?:[.,][0-9]+)?\s*(?:[KMGT]i?B|bytes?)\b`)
var archiveName = regexp.MustCompile(`(?i)[^\s<>"/]+\.(?:zip|7z|rar|tar|gz|rwpkg)\b`)

func genericFileLabel(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "download this file", "download", "download file", "download files", "\u4e0b\u8f7d\u6587\u4ef6":
		return true
	}
	return false
}

func fileMetadata(link *html.Node, fallback string) (name, size, date string) {
	name = strings.TrimSpace(attr(link, "download"))
	if name == "" {
		name = strings.TrimSpace(attr(link, "data-filename"))
	}
	row := link
	for p := link.Parent; p != nil && p.Data != "body"; p = p.Parent {
		// Never take a filename or date from another file in the selector.
		if p.Data == "li" || p.Data == "tr" || hasClass(p, "ipsDataItem") || attr(p, "data-fileid") != "" {
			row = p
			break
		}
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if name == "" && attr(n, "data-filename") != "" {
				name = attr(n, "data-filename")
			}
			if name == "" && (n.Data == "h3" || n.Data == "h4" || strings.EqualFold(attr(n, "data-role"), "filename") || strings.Contains(attr(n, "class"), "ipsDataItem_title")) {
				text := strings.Join(strings.Fields(nodeText(n)), " ")
				if !genericFileLabel(text) {
					name = text
				}
			}
			if size == "" {
				size = attr(n, "data-size")
			}
			if date == "" && n.Data == "time" {
				date = attr(n, "datetime")
				if date == "" {
					date = strings.TrimSpace(nodeText(n))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(row)
	text := strings.Join(strings.Fields(nodeText(row)), " ")
	if name == "" {
		name = archiveName.FindString(text)
	}
	if size == "" {
		size = fileSize.FindString(text)
	}
	if name == "" {
		name = fallback
	}
	return
}

func hasClass(n *html.Node, name string) bool {
	for _, class := range strings.Fields(attr(n, "class")) {
		if class == name {
			return true
		}
	}
	return false
}
