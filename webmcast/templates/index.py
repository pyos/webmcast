render = lambda request: DOCTYPE + html(
  head(
    meta(charset='utf-8'),
    link(rel="stylesheet", href="/static/css/uikit.min.css"),
    link(rel="stylesheet", href="/static/css/layout.css"),
    title('webmcast'),
  ),
  body(
    div.uk_block(
      div.uk_container.uk_container_center(
        h1.uk_heading_large('Hello, World!'),
        h2('Wow.'),
      ),
    ),
    script(src="/static/js/jquery.min.js"),
    script(src="/static/js/uikit.min.js"),
  ),
)
