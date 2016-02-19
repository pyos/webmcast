render = lambda request, code, message='': DOCTYPE + html(
  head(
    meta(charset="utf-8"),
    link(rel="stylesheet", href="/static/css/uikit.min.css"),
    link(rel="stylesheet", href="/static/css/layout.css"),
    title('webmcast / ', code),
  ),
  body.uk_flex.uk_flex_column(
    div.uk_block.uk_block_large.uk_flex_item_auto(
      div.uk_container.uk_container_center(
        h1.uk_heading_large(code,
          small.uk_text_danger(
            '&larr;', { 400: 'bad request',
                        403: 'FOREBODEN',
                        404: 'not found',
                        405: 'not doing that',
                        418: "i'm a little teapot",
                        500: "couldn't do that" }.get(code, '???'),
          ),
        ),
        h2(message),
      ),
    ),
    div.uk_block.uk_flex_item_none(
      div.uk_container.uk_container_center(
        h3('Try ', { 400: 'sending something nice instead',
                     403: 'not being a dirty hacker',
                     405: 'a different course of action',
                     418: 'asking for tea',
                     500: '<a href="https://github.com/pyos/webmcast">submitting a bug report</a>',
                   }.get(code, '<a href="/">starting over</a>'), '.'
        ),
      ),
    ),
  ),
)
