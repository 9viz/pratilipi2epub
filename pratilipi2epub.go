// -*- compile-command: "go build pratilipi2epub.go"; -*-
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-rod/rod"
	"golang.org/x/net/html"
	"io/ioutil"
	"net/http"
	nurl "net/url"
	"strconv"
	"strings"
	"time"
	"os"
	"archive/zip"
)

type QueryVarWhere struct {
	Slug string `json:"seriesSlug"`
}

type QueryVarPage struct {
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

type QueryVar struct {
	Where QueryVarWhere `json:"where"`
	Page  QueryVarPage  `json:"page"`
}

type Query struct {
	OperationName string   `json:"operationName"`
	Variables     QueryVar `json:"variables"`
	Query         string   `json:"query"`
}

// 500 - probably means malformed headers
// 400 - Probably an issue with the POST data
var GRAPHQLURL string = "https://tamil.pratilipi.com/graphql"
var GRAPHQLHEADER map[string][]string = map[string][]string{
	"Apollographql-client-name":    {"WEB_prod"},
	"Apollographql-client-version": {"1.0.0"},
	"Content-type":                 {"application/json"},
}

type AuthorResp struct {
	Data struct {
		GetSeries struct {
			Series struct {
				PublishedParts struct {
					Parts []struct {
						Pratilipi struct {
							Author struct {
								DisplayName string `json:"displayName"`
							} `json:"author"`
						} `json:"pratilipi"`
					} `json:"parts"`
				} `json:"publishedParts"`
			} `json:"series"`
		} `json:"getSeries"`
	} `json:"data"`
}

// Get the author of the series represented by the slug SLUG.
func getAuthor(slug string) string {
	where := QueryVarWhere{slug}
	page := QueryVarPage{1, "1"}
	query := "query getSeriesPartsPaginatedBySlug($where: GetSeriesInput!, $page: LimitCursorPageInput) { " +
		"getSeries(where: $where) { " +
		"series { " +
		"publishedParts(page: $page) { " + "parts { pratilipi { author { displayName } } }" +
		" } } } }"
	d := Query{"getSeriesPartsPaginatedBySlug", QueryVar{where, page}, query}
	j, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	req, _ := http.NewRequest("POST", GRAPHQLURL, bytes.NewReader(j))
	req.Header = GRAPHQLHEADER
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	client.CloseIdleConnections()
	body, _ := ioutil.ReadAll(resp.Body)
	var data AuthorResp
	err = json.Unmarshal(body, &data)
	if err != nil {
		panic(err)
	}
	return data.Data.GetSeries.Series.PublishedParts.Parts[0].Pratilipi.Author.DisplayName
}

type SeriesPartResp struct {
	Data struct {
		GetSeries struct {
			Series struct {
				PublishedParts struct {
					Parts []struct {
						Pratilipi struct {
							Title       string `json:"title"`
							ReadPageURL string `json:"readPageUrl"`
							PublishedAt int64  `json:"publishedAt"`
							PratilipiEarlyAccess struct {
								IsEarlyAccess bool `json:"isEarlyAccess"`
							} `json:"pratilipiEarlyAccess"`
						} `json:"pratilipi"`
					} `json:"parts"`
				} `json:"publishedParts"`
			} `json:"series"`
		} `json:"getSeries"`
	} `json:"data"`
}

// Get the series part for the slug SLUG, upto LIMIT, from CURSOR.
func getSeriesPart(slug string, limit int, cursor int) SeriesPartResp {
	query := "query getSeriesPartsPaginatedBySlug($where: GetSeriesInput!, $page: LimitCursorPageInput) { " +
		"getSeries(where: $where) { " +
		"series { " +
		"publishedParts(page: $page) { " +
		"parts { " +
		"pratilipi { " +
		"title " +
		"readPageUrl " +
		"publishedAt " +
		"pratilipiEarlyAccess { isEarlyAccess } " +
		"} } } } } }"
	querydata := Query{
		"getSeriesPartsPaginatedBySlug",
		QueryVar{
			QueryVarWhere{slug},
			QueryVarPage{limit, strconv.Itoa(cursor)},
		},
		query,
	}
	j, err := json.Marshal(querydata)
	if err != nil {
		panic(err)
	}
	req, _ := http.NewRequest("POST", GRAPHQLURL, bytes.NewReader(j))
	req.Header = GRAPHQLHEADER
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	client.CloseIdleConnections()
	body, _ := ioutil.ReadAll(resp.Body)
	var data SeriesPartResp
	err = json.Unmarshal(body, &data)
	if err != nil {
		panic(err)
	}
	return data
}

// Extract [ title, readPageUrl, publishedAt formatted ] for all
// chapters in the slug SLUG.
func extractLinks(slug string) [][]string {
	var url [][]string
	fetchp := true
	limit := 15
	cursor := 0
	for fetchp {
		data := getSeriesPart(slug, limit, cursor)
		d := data.Data.GetSeries.Series.PublishedParts.Parts
		if len(d) != limit {
			fetchp = false
		}
		for _, i := range d {
			if i.Pratilipi.PratilipiEarlyAccess.IsEarlyAccess {
				fetchp = false
				break
			}
			i := i.Pratilipi
			t := time.Unix(i.PublishedAt/1000, 0)
			t = t.Local()
			url = append(url, []string{
				i.Title,
				i.ReadPageURL,
				t.Format("Jan 18, 2020"),
			})
		}
		cursor += limit
	}
	return url
}

// Extra data that are needed for the chapter.
type ContentExtra struct {
	FileName    string
	FileId      string
	FileContent []byte
	Mimetype    string
}

// Return the story content with story details stored in DATA.
// DATA follows the format of return value of extractLinks.
// BROWSER is the rod Browser object used to fetch the pages.
// N denotes the chapter's number.
// The function might return another nested list of the struct
// ContentExtra.
// The function returns ("", nil) if there is no div .content-section
// element could not be found.
func getContent(browser *rod.Browser, data []string, N int) (string, []ContentExtra) {
	page := browser.MustPage("https://tamil.pratilipi.com" + data[1])
	defer page.MustClose()

	page.MustWaitLoad()
	var contentBuilder strings.Builder
	var extra []ContentExtra

	// Check if it contains the relevant div section first.
	if has := page.MustHas("div .book-content"); !has {
		return "", nil
	}

	// Preamble.
	contentBuilder.WriteString(`<?xml version="1.0" encoding="UTF-8" ?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml" xml:lang="ta">
  <head>
    <meta http-equiv="Content-Type" content="application/xhtml+xml; charset=utf-8" />
    <title>`)
	contentBuilder.WriteString(data[0])
	contentBuilder.WriteString(`</title>
    <link rel="stylesheet" href="css/main.css" type="text/css" />
  </head>
  <body>`)

	// First, find the h1 tag.
	contentBuilder.WriteString("\n")
	contentBuilder.WriteString(page.MustElement("div .book-content").MustElement("h1").MustHTML())
	contentBuilder.WriteString("\n\n")

	// The idea of using a tokenizer rather than html.Parser came
	// from https://zetcode.com/golang/net-html/.
	contentHtml := page.MustElement("div .content-section").MustHTML()
	token := html.NewTokenizer(strings.NewReader(contentHtml))
	dataSuggestion := false
	imgCounter := 1
OLoop:
	for {
		tk := token.Next()
		tok := token.Token()
		switch {
		case tk == html.ErrorToken:
			break OLoop
		case tk == html.StartTagToken && tok.Data == "span":
			for _, i := range tok.Attr {
				if i.Key == "data-suggestions" {
					contentBuilder.WriteString(i.Val)
					dataSuggestion = true
					break
				}
			}
			if !dataSuggestion {
				contentBuilder.WriteString(html.UnescapeString(tok.String()))
			}
		case tk == html.EndTagToken && tok.Data == "span":
			if !dataSuggestion {
				contentBuilder.WriteString(html.UnescapeString(tok.String()))
			}
			dataSuggestion = false
		// TODO:
		case tok.Data == "img":
			imgDone := false
			for _, i := range tok.Attr {
				if i.Key == "src" {
					res, err := page.GetResource(html.UnescapeString(i.Val))
					if err != nil {
						break
					}
					mimetype := http.DetectContentType(res)
					// ../Images because src is relative to Text/.
					contentBuilder.WriteString("<img src='../Images/ch")
					contentBuilder.WriteString(strconv.Itoa(N))
					contentBuilder.WriteString("_img")
					contentBuilder.WriteString(strconv.Itoa(imgCounter))
					contentBuilder.WriteString("' />")
					extra = append(extra, ContentExtra{
						"Images/ch" + strconv.Itoa(N) + "_img" + strconv.Itoa(imgCounter),
						"ch" + strconv.Itoa(N) + "img" + strconv.Itoa(imgCounter),
						res,
						mimetype,
					})
					imgCounter += 1
					imgDone = true
					break
				}
			}
			if !imgDone {
				contentBuilder.WriteString(html.UnescapeString(tok.String()))
			}
		default:
			contentBuilder.WriteString(html.UnescapeString(tok.String()))
		}
	}

	// End preamble.
	contentBuilder.WriteString(`</body>
</html>
`)

	return contentBuilder.String(), extra
}

// >>>>>>>>>> EPUB SECTION <<<<<<<<<<<<<

// Epub is a zip file with the following required files:
// . mimetype - which should be the first and contains the mimetype of
//   the file---application/epub+zip.
// . META-INF/container.xml - this file tells where the "root file" is.
//   Root file is where the rest of the files listed below are stored.
//   Usually the location is "OEBPS".
// . OEBPS/content.opf - this file lists all the files used by the epub.
// . toc.ncx - this has the order of the files to be opened aka Table
//   of Contents.
// The epub wikipedia page has a very nice summary: https://en.wikipedia.org/wiki/EPUB#Version_2.0.1
// Opening the epub archive yourself will also give you a good idea of
// what needs to be done.

// Return the file contents of the content.opf file for the series.
// AUTHOR is the author of the series, TITLE is the name of the
// series, IDENTIFIER is the value of unique-identifier for the
// series, FILES is a list of [ FTITLE, FILENAME ] where FTITLE is the
// title of the chapter, and FILENAME is the filename of the chapter.
// FILES may also contain [ FTITLE, FILENAME, MIMETYPE ] in which case
// FTITLE is used as the value of `id' attribute in the item entries,
// FILENAME is taken literally (i.e., not under Text/) and MIMETYPE is
// used for that instead of application/xhtml+xml.
// The unique identifier used will always be "BookId" and the chapters
// will be stored under OEBPS/Text/.
func prepareContentOpf(author, identifier, title string, files [][]string) []byte {
	var content strings.Builder

	// Header.
	content.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<package version="2.0" unique-identifier="BookId" xmlns="http://www.idpf.org/2007/opf">`)

	// First do the metadata section.
	content.WriteString(`<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"  xmlns:opf="http://www.idpf.org/2007/opf">
`)
	content.WriteString("<dc:creator>")
	content.WriteString(author)
	content.WriteString("</dc:creator>\n")
	content.WriteString("<dc:identifier id=\"BookId\">")
	content.WriteString(identifier)
	content.WriteString("</dc:identifier>\n")
	content.WriteString("<dc:language>ta</dc:language>\n")
	content.WriteString("<dc:title>")
	content.WriteString(title)
	content.WriteString("</dc:title>\n")
	content.WriteString(`<dc:date opf:event="modification" xmlns:opf="http://www.idpf.org/2007/opf">`)
	content.WriteString(time.Now().Format("2022-01-18"))
	content.WriteString("</dc:date>\n")
	content.WriteString("</metadata>\n\n")

	// Manifest section.
	content.WriteString("<manifest>")
	n := 1
	for _, i := range files {
		if len(i) != 3 {
			content.WriteString("\n<item id='Chapter")
			content.WriteString(strconv.Itoa(n))
			content.WriteString("' href='Text/")
			content.WriteString(i[1])
			content.WriteString("' media-type='application/xhtml+xml'/>")
			n += 1
		} else {
			content.WriteString("\n<item id='")
			content.WriteString(i[0])
			content.WriteString("' href='")
			content.WriteString(i[1])
			content.WriteString("' media-type='")
			content.WriteString(i[2])
			content.WriteString("' />")
		}
	}
	content.WriteString("\n<item id='ncx' href='toc.ncx' media-type='application/x-dtbncx+xml'/>\n</manifest>\n")

	// Spine section.
	content.WriteString("\n<spine toc='ncx'>")
	for i := 1; i <= n; i++ {
		content.WriteString("\n<itemref idref='Chapter")
		content.WriteString(strconv.Itoa(i))
		content.WriteString("'/>")
	}
	content.WriteString("\n</spine>\n")

	// No optional guide section.
	content.WriteString("</package>\n")
	return []byte(content.String())
}

// Return the file contents of the toc.ncx file for the series.
// Arguments have the same meaning as for prepareContentOpf.  FILES
// with three elements (for content other than xhtml files) are simply
// ignored.
func prepareTocNcx(author, identifer, title string, files [][]string) []byte {
	var content strings.Builder

	// Header.
	content.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE ncx PUBLIC "-//NISO//DTD ncx 2005-1//EN" "http://www.daisy.org/z3986/2005/ncx-2005-1.dtd">

<ncx version="2005-1" xml:lang="ta" xmlns="http://www.daisy.org/z3986/2005/ncx/">
  <head>
    <meta name="dtb:uid" content="`)
	content.WriteString(identifer)
	content.WriteString(`"/>
    <meta name="dtb:depth" content="1"/>
    <meta name="dtb:totalPageCount" content="0"/>
    <meta name="dtb:maxPageNumber" content="0"/>
  </head>

  <docTitle><text>`)
	content.WriteString(title)
	content.WriteString(`</text></docTitle>
  <docAuthor><text>`)
	content.WriteString(author)
	content.WriteString(`</text></docAuthor>
  <navMap>`)

	n := 1
	// Now for the nested structure.
	for _, i := range files {
		// Non-xhtml files.
		if len(i) == 3 {
			continue
		}
		content.WriteString("\n<navPoint id='Chapter")
		content.WriteString(strconv.Itoa(n))
		content.WriteString("' playOrder='")
		content.WriteString(strconv.Itoa(n))
		content.WriteString("'>\n")

		content.WriteString("<navLabel><text>")
		content.WriteString(i[0])
		content.WriteString("</text></navLabel>\n")
		content.WriteString("<content src='Text/")
		content.WriteString(i[1])
		content.WriteString("' />\n</navPoint>")
		n += 1
	}

	content.WriteString("\n</navMap>\n</ncx>\n")
	return []byte(content.String())
}

// Return the file contents of the container.xml file.
func prepareContainerXml() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
   </rootfiles>
</container>
`)
}

// Return the file contents of the mimetype file.
func prepareMimetype() []byte {
	return []byte("application/epub+zip\n")
}

// Return the slug part of the URL URL.
func getSlug(url string) string {
	u, err := nurl.Parse(url)
	if err != nil {
		panic(err)
	}
	s := strings.Split(u.Path, "/")
	return s[len(s)-1]
}

// Get the series title of the story in URL URL.
func getSeriesTitle(browser *rod.Browser, url string) string {
	page := browser.MustPage(url)
	page.MustWaitLoad()
	defer page.Close()
	return page.MustElement("h1.title").MustText()
}

type Zipfile struct {
	Filename string
	Content []byte
}

// Do everything for the book URL URL.
func do(browser *rod.Browser, url string) {
	slug := getSlug(url)
	chapterLinks := extractLinks(slug)
	// mimetype file must be the first one in the archive.
	files := []Zipfile{
		{"mimetype", prepareMimetype()},
		{"META-INF/container.xml", prepareContainerXml()},
	}
	// [ FTITLE, FILENAME ] or [ FTITLE, FILENAME, MIMETYPE ]
	var filesForEpub [][]string

	fmt.Println("Total number of", len(chapterLinks), "chapters to fetch")

	for n, ch := range chapterLinks {
		fmt.Println("Fetching content for chapter", n+1, "via", ch[1])
		content, extra := getContent(browser, ch, n+1)
		if content == "" && extra == nil {
			fmt.Println(ch[1], "does not have div .book-content, skipping...")
			continue
		}
		fname := "Chapter" + strconv.Itoa(n+1) + ".xhtml"
		files = append(files, Zipfile{
			"OEBPS/Text/" + fname,
			[]byte(content),
		})
		filesForEpub = append(filesForEpub, []string{
			ch[0],
			fname,
		})
		for _, i := range extra {
			files = append(files, Zipfile{
				"OEBPS/" + i.FileName,
				i.FileContent,
			})
			filesForEpub = append(filesForEpub, []string{
				i.FileId,
				i.FileName,
				i.Mimetype,
			})
		}
	}

	author := getAuthor(slug)
	title := getSeriesTitle(browser, url)

	fmt.Println("Creating content.opf and toc.ncx files")
	files = append(files, Zipfile{
		"OEBPS/content.opf",
		prepareContentOpf(author, slug, title, filesForEpub),
	})
	files = append(files, Zipfile{
		"OEBPS/toc.ncx",
		prepareTocNcx(author, slug, title, filesForEpub),
	})

	fmt.Println("Creating the epub file")
	epubFile, err := os.Create(slug + ".epub")
	if err != nil {
		panic(err)
	}
	defer epubFile.Close()

	w := zip.NewWriter(epubFile)
	defer w.Close()

	for _, file := range files {
		f, err := w.Create(file.Filename)
		if err != nil {
			panic(err)
		}
		_, err = f.Write(file.Content)
		if err != nil {
			panic(err)
		}
	}
}

func main() {
	if (len(os.Args) == 1) {
		fmt.Println("usage: pratilipi2epub LINKS...")
		os.Exit(1)
	}

	browser := rod.New().MustConnect().NoDefaultDevice()
	defer browser.MustClose()

	for _, l := range os.Args[1:] {
		do(browser, l)
	}
}
