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
