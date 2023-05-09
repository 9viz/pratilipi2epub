#!/usr/bin/env python3
# A python script to extract a (Tamil) pratilipi series to an epub
# file.

import urllib.request as req
import urllib.error
from urllib.parse import urlparse
import json
import time
import locale
import re

GRAPHQL_SERIES_PARTS = """query getSeriesPartsPaginatedBySlug($where: GetSeriesInput!, $page: LimitCursorPageInput) {\
  getSeries(where: $where) {\
    series {\
      seriesId\
      id\
      publishedParts(page: $page) {\
        cursor\
        id\
        parts {\
          id\
          pratilipi {\
            title\
            readPageUrl\
            publishedAt\
          }\
        }\
      }\
    }\
  }\
}"""

BASE_URL = "https://tamil.pratilipi.com"

def get_slug(uri):
    """Get the slug part from the series url URI."""
    return urlparse(uri).path.split("/")[-1]

def get_series_part(slug, limit=20, cursor=0):
    """Get series part from CURSOR to CURSOR+LIMIT.
    SLUG is the URI component of the series that identifies it.
    """
    global GRAPHQL_SERIES_PARTS
    query = {
        "operationName": "getSeriesPartsPaginatedBySlug",
        "variables": {
            "where": {
                "seriesSlug": slug
            },
            "page": {
                "limit": limit,
                "cursor": f"{cursor}",
            },
        },
        "query": GRAPHQL_SERIES_PARTS,
    }
    query = json.dumps(query, separators=(",", ":"))
    r = req.Request(
        "https://tamil.pratilipi.com/graphql",
        headers={
            "Apollographql-client-name": "WEB_prod",
            "Apollographql-client-version": "1.0.0",
            "Content-type": "application/json"
        },
        data=query.encode()
    )
    return json.loads(req.urlopen(r).read())

def get_author(slug):
    """Return the author for the slug SLUG."""

    query = {
        "operationName": "getSeriesPartsPaginatedBySlug",
        "variables": {
            "where": {
                "seriesSlug": slug
            },
            "page": {
                "limit": 1,
                "cursor": "1",
            },
        },
        "query": """query getSeriesPartsPaginatedBySlug($where: GetSeriesInput!, $page: LimitCursorPageInput) {\
  getSeries(where: $where) {\
    series {\
      publishedParts(page: $page) {\
        parts {\
          pratilipi {\
            author { displayName }
          }\
        }\
      }\
    }\
  }\
}""",
    }
    query = json.dumps(query, separators=(",", ":"))
    r = req.Request(
        "https://tamil.pratilipi.com/graphql",
        headers={
            "Apollographql-client-name": "WEB_prod",
            "Apollographql-client-version": "1.0.0",
            "Content-type": "application/json"
        },
        data=query.encode()
    )
    data = json.loads(req.urlopen(r).read())
    data = data["data"]["getSeries"]["series"]["publishedParts"]["parts"]
    return data[0]["pratilipi"]["author"]["displayName"]

# TODO: Include author.
locale.setlocale(locale.LC_TIME, "ta_IN")
def extract_links(slug):
    """Extract `readPageUrl' links for all chapters in SLUG.
    A nested list is returned in the published were each list is of
    the form [ `title', `readPageUrl', `publishedAt' formatted ].

    """
    url = []
    fetchp = True
    limit = 15
    cursor = 0
    while fetchp:
        data = get_series_part(slug, limit, cursor)
        d = data["data"]["getSeries"]["series"]["publishedParts"]["parts"]
        # If the amount of the data returned is less than requested,
        # then end the loop.
        if len(d) != limit:
            fetchp = False

        for i in d:
            i = i["pratilipi"]
            t = i["publishedAt"]/1000
            t = time.localtime(t)
            t = time.strftime("%d %B %Y", t)
            url.append([ i["title"], i["readPageUrl"], t ])
        cursor += limit

    return url


# Epub is a zip file with the following required files:
# . mimetype - which should be the first and contains the mimetype of
#   the file---application/epub+zip.
# . META-INF/container.xml - this file tells where the "root file" is.
#   Root file is where the rest of the files listed below are stored.
#   Usually the location is "OEBPS".
# . OEBPS/content.opf - this file lists all the files used by the epub.
# . toc.ncx - this has the order of the files to be opened aka Table
#   of Contents.
# The epub wikipedia page has a very nice summary: https://en.wikipedia.org/wiki/EPUB#Version_2.0.1
# Opening the epub archive yourself will also give you a good idea of
# what needs to be done.

def prepare_content_opf(author, identifier, title, files):
    """Return the file content of content.opf as a string.
    AUTHOR is the author of the story.  IDENTIFIER is an unique id
    representing the story.  TITLE is the title/name of the story, and
    FILES is a list of the HTML files.  FILES should NOT include other
    files that are required/expected to be present in an epub file.
    The unique identifier used for the epub file will always be
    "BookId".
    The FILES will be stored under OEBPS/Text/.
    """
    def prep_metadata():
        date = time.strftime("%Y-%m-%d")
        return f"""
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/"  xmlns:opf="http://www.idpf.org/2007/opf">
 <dc:creator>{author}</dc:creator>
 <dc:identifier id="BookId">{identifier}</dc:identifier>
 <dc:language>ta</dc:language>
 <dc:title>{title}</dc:title>
 <dc:date opf:event="modification" xmlns:opf="http://www.idpf.org/2007/opf">{date}</dc:date>
</metadata>
        """

    def prep_manifest():
        manifest = "<manifest>"
        for n,i in enumerate(files):
            # Filetype needs to xhtml.
            manifest += f"""\n<item id='Chapter{n+1}' href="Text/{i}" media-type='application/xhtml+xml'/>"""
        manifest += """\n<item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>\n<manifest/>"""
        return manifest

    def prep_spine():
        spine = """<spine toc="ncx">"""
        for i in range(len(files)):
            spine += f"\n<itemref idref='Chapter{i+1}'/>"
        spine += "\n</spine>"
        return spine

    content = """<?xml version="1.0" encoding="utf-8">
<package version="2.0" unique-identifier="BookId" xmlns="http://www.idpf.org/2007/opf">
    """

    # We don't need <guide> since we don't have a cover page or
    # anything special like that.
    content += prep_metadata() + "\n\n"
    content += prep_manifest() + "\n\n"
    content += prep_spine() + "\n\n"
    content += "</package>"

    return content

def prepare_toc_ncx(author, identifier, title, files):
    toc = f"""<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE ncx PUBLIC "-//NISO//DTD ncx 2005-1//EN"
"http://www.daisy.org/z3986/2005/ncx-2005-1.dtd">

<ncx version="2005-1" xml:lang="ta" xmlns="http://www.daisy.org/z3986/2005/ncx/">
  <head>
    <meta name="dtb:uid" content="{identifier}"/>
    <meta name="dtb:depth" content="1"/>
    <meta name="dtb:totalPageCount" content="0"/>
    <meta name="dtb:maxPageNumber" content="0"/>
  </head>

  <docTitle><text>{title}</text></docTitle>
  <docAuthor><text>{author}</text></docAuthor>
  <navMap>
    """
    for n,i in enumerate(files):
        toc += f"""\n<navPoint id="Chapter{n+1}" playOrder="{n+1}">
        <navLabel><text>Chapter {n+1}</text></navLabel>
        <content src="Text/{i}" />
        </navPoint>"""
    toc += "</navMap>\n</ncx>"
    return toc

def prepare_container_xml():
    return """<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
   </rootfiles>
</container>
    """

def prepare_mimetype():
    return "application/epub+zip"
