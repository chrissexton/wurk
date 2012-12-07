WURK
----

Wurk provides the simplest use case of the Werc framework from cat-v.org. I was
mostly interested in figuring out Go's web tools, but had the side goal of
producing a very bare bones Markdown engine that could be hacked to my
satisfaction.

Hosting sites
-------------

Each website hosted by Wurk gets its own directory corresponding to its
hostname. For instance, example.com is included, but this could be linked to
127.0.0.1:6969 for testing locally.

Website directories contain a pub directory which is a freeform dump of Markdown
and regular files. All markdown should end in a .md extension. Directories will
naturally create a site heirarchy. A templates directory contains the look and
feel of the site in Go's html/template format.
