"use strict";  /* global init */


let markup = {
    blocks: [
        { k: 'code',   e: /^ {4}|^\t/g            },
        { k: 'rule',   e: /^\s*(?:[*-]\s*){3,}$/g },
        { k: 'ol',     e: /^\s*\d+\.\s+/g         },
        { k: 'ul',     e: /^\s*[*+-]\s+/g         },
        { k: 'h3',     e: /^\s*###\s*/g           },
        { k: 'h2',     e: /^\s*##\s*/g            },
        { k: 'h1',     e: /^\s*#\s*/g             },
        { k: 'table',  e: /^\s*\|/g               },
        { k: 'quote',  e: /^\s*>/g                },
        { k: null,     e: /^\s*$/g                },
        { k: 'p',      e: /^\s*/g                 },
    ],

    inline: [
        { k: 'code',      e: /(`+)(.+?)\1/g          },  // -> 2
        { k: 'bold',      e: /\*\*((?:\\?.)+?)\*\*/g },  // -> 1
        { k: 'italic',    e: /\*((?:\\?.)+?)\*/g     },  // -> 1
        { k: 'strike',    e: /~~((?:\\?.)+?)~~/g     },  // -> 1
        { k: 'invert',    e: /%%((?:\\?.)+?)%%/g     },  // -> 1
        { k: 'hyperlink', e: /\b[a-z][a-z0-9+\.-]*:(?:[,\.?]?[^\s(<>)"\',\.?%]|%[0-9a-f]{2}|\([^\s(<>)"\']+\))+/g },  // -> 0
        { k: 'namedlink', e: /\[(.*?)\]\(((?:[^()]+|\(.*?\)|[^)])*)\)/g },  // -> (1 = text, 2 = href)
        { k: 'text',      e: /[\w-]*[^\W_-]\s*/g     },  // -> 0
        { k: 'escape',    e: /\\?(.)/g               },  // -> 1
    ],

    parse: (text) => {
        let nl = /$/gm;
        let out = '';
        let key = null;
        let lines = [];
        let trimmed = [];

        for (; text !== ""; text = text.substr(nl.lastIndex + 1)) {
            nl.lastIndex = 0;
            nl.test(text);
            let line = text.substr(0, nl.lastIndex);

            for (let b of markup.blocks) {
                b.e.lastIndex = 0;
                let groups = b.e.exec(line);
                if (groups === null)
                    continue;

                if (b.k !== key) {
                    out += markup.group(key, lines, trimmed);
                    key = b.k;
                    lines = [];
                    trimmed = [];
                }

                lines.push(line);
                trimmed.push(line.substr(b.e.lastIndex));
                break;
            }
        }

        return out + markup.group(key, lines, trimmed);
    },

    group: (key, lines, trimmed) => {
        if (key === null || lines.length === 0)
            return '';
        if (key === 'quote')
            return '<blockquote>' + markup.parse(trimmed.join('\n')) + '</blockquote>';
        if (key === 'code')
            return '<pre>' + markup.escape(trimmed.join('\n')) + '</pre>';
        if (key === 'ul' || key == 'ol')
            return '<' + key + '><li>' + trimmed.map(x => markup.parseInline(markup.escape(x))).join('</li><li>') + '</li></' + key + '>';
        if (key === 'table')
            return '<table>' + trimmed.map(x => '<tr><td>' + x.replace(/\|\s*$/, '').replace(/\|/g, '</td><td>') + '</td></tr>').join('') + '</table>';
        return '<' + key + '>' + markup.parseInline(markup.escape(trimmed.join('\n'))) + '</' + key + '>';
    },

    'escape': (x) => {
        return x.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
    },

    parseInline: (x) => {
        let out = '';
        while (x !== "") {
            let best = {start: x.length};

            for (let i of markup.inline) {
                i.e.lastIndex = 0;
                let groups = i.e.exec(x);
                if (groups !== null && i.e.lastIndex - groups[0].length < best.start) {
                    best = {key: i.k, start: i.e.lastIndex - groups[0].length, end: i.e.lastIndex, groups};
                }
            }

            out += x.substr(0, best.start);
            if (best.key === 'text')
                out += best.groups[0];
            else if (best.key === 'escape')
                out += best.groups[1];
            else if (best.key === 'bold')
                out += '<strong>' + markup.parseInline(best.groups[1]) + '</strong>';
            else if (best.key === 'italic')
                out += '<em>' + markup.parseInline(best.groups[1]) + '</em>';
            else if (best.key === 'invert')
                out += '<span class="spoiler">' + markup.parseInline(best.groups[1]) + '</span>';
            else if (best.key === 'strike')
                out += '<del>' + markup.parseInline(best.groups[1]) + '</del>';
            else if (best.key === 'code')
                out += '<code>' + best.groups[2] + '</code>';
            else if (best.key === 'hyperlink')
                out += '<a href="' + best.groups[0] + '" target="_blank">' + best.groups[0] + '</a>';
            else if (best.key === 'namedlink')
                out += '<a href="' + best.groups[2] + '" target="_blank">' + best.groups[1] + '</a>';
            else
                out += x.substr(best.start, best.end);
            x = x.substr(best.end);
        }
        return out + x;
    },
};


init['[data-markup]'] = (e) => {
    let marked = document.createElement('div');
    marked.setAttribute('data-markup-html', '');
    marked.innerHTML = markup.parse(e.textContent);
    new MutationObserver(() => { marked.innerHTML = markup.parse(e.textContent); })
        .observe(e, {childList: true, characterData: true});
    e.parentElement.insertBefore(marked, e);
};
