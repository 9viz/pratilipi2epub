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
            author { id displayName }
          }\
        }\
      }\
    }\
  }\
}"""

ACCESS_TOKEN = ""

def access_token():
    global ACCESS_TOKEN
    if not ACCESS_TOKEN:
        r = req.urlopen("https://gamma.pratilipi.com/api/user/accesstoken")
        r = r.read()
        ACCESS_TOKEN = json.loads(r)["accessToken"]
    return ACCESS_TOKEN

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

# TODO: Include author.
locale.setlocale(locale.LC_TIME, "ta_IN")
def extract_links(slug):
    """Extract `readPageUrl' links for all chapters in SLUG.
    A dictionary is returned whose key is the title of the chapter and
    value is a list [ `readPageUrl', `publishedAt' formatted ].

    """
    url = {}
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
            url[i["title"]] = [ i["readPageUrl"], t ]
        cursor += limit

    return url

RE_SCRIPT = re.compile(r"window\.__NUXT")

# The content is stored in a custom encoded format.  The encoded
# content is in the middle of a huge one line JS snippet.
# The relevant part starts from appolo:{"....  and the actual content
# is in a "content" field with a ({\"isFullcontent\": right next to
# it.
RE_CONTENT = re.compile(
    r'.*apollo:{.*content\({\\"isFullContent\\":(?:true|false)}\)":"([^"]+).*'
)

def get_content(uri):
    """Get content for chapter specified URI component URI.
    URL used is https://tamil.pratilipi.com + URI.

    """
    soup = bs4.BeautifulSoup(req.urlopen("https://tamil.pratilipi.com" + uri))
    script = soup.find("body").find(name="script", string=RE_SCRIPT)
    content = re.match(RE_CONTENT, script).group(1)
    # The content itself is a JS-escaped string so we need to take
    # care of \\uXXXX parts hence this byte dance.
    content = bytes(content,"ascii").decode("unicode_script")
    return decode_content(content)


# Epub is a zip file with the following required files:
# . mimetype - which should be the first and contains the mimetype of
#   the file---application/epub+zip.
# . META-INF/container.xml - this file tells where the "root file" is.
#   Root file is where the rest of the files listed below are stored.
#   Usually the location is "OEBPS".
# . OEBPS/content.opf - this file lists all the files used by the epub.
# . toc.ncx - this has the order of the files to be opened aka Table
#   of Contents.
# This wikipedia page has a very nice summary -
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
 <dc:identifier id="BookId" opf:scheme="?????">{identifier}</dc:identifier>
 <dc:language>ta</dc:language>
 <dc:title>{title}</dc:title>
 <dc:subject/>
 <meta name="cover" content="Cover_v3.jpg"/> ??????
 <meta content="1.8.0" name="Sigil version"/> ?????
 <dc:date opf:event="modification" xmlns:opf="http://www.idpf.org/2007/opf">{date}</dc:date>
</metadata>
        """

    def prep_manifest():
        manifest = """<manifest>
  <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
        """
        for i in files:
            manifest += f"\n<item id='{i}' href="Text/{i}" media-type='application/html'/>"
        manifest += "\n<manifest/>"
        return manifest

    def prep_spine():
        spine = """<spine toc="ncx">"""
        for i in files:
            spine += f"\n<itemref idref='{i}'/>"
        spine += "\n</spine>"
        return spine

    content = """<?xml version="1.0" encoding="utf-8">
<package version="2.0" unique-identifier="BookId" xmlns="http://www.idpf.org/2007/opf">
    """

    def prep_guide():
        return ""

    content += prep_metadata() + "\n\n"
    content += prep_manifest() + "\n\n"
    content += prep_spine() + "\n\n"
    content += prep_guide() + "\n\n</package>"

    return content

def prepare_toc_ncx(identifier, files):
    return ""

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
