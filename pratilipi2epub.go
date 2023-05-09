package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"
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
		fmt.Println(fetchp)
		for _, i := range d {
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

// Prepare the content.opf file for the series.  AUTHOR is the author
// of the series, TITLE is the name of the series, IDENTIFIER is the value of
// unique-identifier for the series, FILES is a list of [ FTITLE,
// FILENAME ] where FTITLE is the title of the chapter, and FILENAME
// is the filename of the chapter.
// The unique identifier used will always be "BookId" and the chapters
// will be stored under OEBPS/Text/.
func prepareContentOpf(author, identifier, title string, files [][]string) string {
	var content strings.Builder

	// Header.
	content.WriteString(`<?xml version="1.0" encoding="utf-8">
<package version="2.0" unique-identifier="BookId" xmlns="http://www.idpf.org/2007/opf">`)

	// First do the metadata section.
	content.WriteString(`<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"  xmlns:opf="http://www.idpf.org/2007/opf">
`)
	content.WriteString("<dc:creator>")
	content.WriteString(author)
	content.WriteString("</dc:creator>\n")
	content.WriteString("<dc:identifier id=\"BookId\">")
	content.WriteString(identifier)
	content.WriteString("</dc:identifer>\n")
	content.WriteString("<dc:language>ta</dc:identifer>\n")
	content.WriteString("<dc:title>")
	content.WriteString(title)
	content.WriteString("</dc:title>\n")
	content.WriteString(`<dc:data opf:event="modification" xmlns:opf="http://www.idpf.org/2007/opf">`)
	content.WriteString(time.Now().Format("2022-01-18"))
	content.WriteString("</dc:date>\n")
	content.WriteString("</metadata>\n\n")

	// Manifest section.
	content.WriteString("<manifest>")
	for n, i := range files {
		content.WriteString("\n<item id='Chapter")
		content.WriteString(strconv.Itoa(n + 1))
		content.WriteString("' href='Text/")
		content.WriteString(i[1])
		content.WriteString("' media-type='application/xhtml+xml'/>")
	}
	content.WriteString("\n<item id='ncx' href='toc.ncx' media-type='application/x-dtbncx+xml'/>\n</manifest>\n")

	// Spine section.
	content.WriteString("\n<spine toc='ncx'>")
	for n, _ := range files {
		content.WriteString("\n<item idref='Chapter")
		content.WriteString(strconv.Itoa(n + 1))
		content.WriteString("'/>")
	}
	content.WriteString("\n</spine>\n")

	content.WriteString("</package>\n")
	return content.String()
}

func prepareTocNcx(author, identifer, title string, files [][]string) string {
	var content strings.Builder

	// Header.
	content.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE ncx PUBLIC "-//NISO//DTD ncx 2005-1//EN"
"http://www.daisy.org/z3986/2005/ncx-2005-1.dtd">

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

	// Now for the nested structure.
	for n, i := range files {
		content.WriteString("\n<navPoint id='Chapter")
		content.WriteString(strconv.Itoa(n + 1))
		content.WriteString("' playOrder='")
		content.WriteString(strconv.Itoa(n + 1))
		content.WriteString("'>\n")

		content.WriteString("<navLabel><text>")
		content.WriteString(i[0])
		content.WriteString("</text></navLabel>\n")
		content.WriteString("<content src='Text/")
		content.WriteString(i[1])
		content.WriteString("' />\n<navPoint>")
	}

	content.WriteString("\n</navMap>\n</ncx>\n")
	return content.String()
}

func prepareContainerXml() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
   </rootfiles>
</container>
`
}

func prepareMimetype() string {
	return "application/epub+zip\n"
}

func main() {
	// fmt.Println(getAuthor("vilaga-viduvena-amirthavarshini-by-sumathij-rajendran-cobmuv7jdcbr"))
	// fmt.Println(getSeriesPart("vilaga-viduvena-amirthavarshini-by-sumathij-rajendran-cobmuv7jdcbr", 1, 1))
	//fmt.Println(extractLinks("vilaga-viduvena-amirthavarshini-by-sumathij-rajendran-cobmuv7jdcbr"))
	fmt.Println(prepareContentOpf("me", "myself", "lmao", [][]string{
		[]string{"loooool", "chapter1.xhtml"},
		[]string{"lmao", "chapter2.xhtml"},
	}))
	fmt.Println(prepareTocNcx("me", "myself", "lmao", [][]string{
		[]string{"loooool", "chapter1.xhtml"},
		[]string{"lmao", "chapter2.xhtml"},
	}))
}
