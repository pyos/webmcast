"use strict";  /* global $init */


let markup = {
    blockRe: [
        { 'pre':   /^ {4}|^\t/g            },
        { 'rule':  /^\s*(?:[*-]\s*){3,}$/g },
        { 'ol':    /^\s*\d+\.\s+/g         },
        { 'ul':    /^\s*[*+-]\s+/g         },
        { 'h3':    /^\s*###\s*/g           },
        { 'h2':    /^\s*##\s*/g            },
        { 'h1':    /^\s*#\s*/g             },
        { 'quote': /^\s*>/g                },
        { 'break': /^\s*$/g                },
        { 'p':     /^\s*/g                 },
    ],

    blockFn: {
        'break': _ => '',
        'rule':  _ => '<hr/>',
        'quote': x => '<blockquote>' + markup.parse(x.join('\n'))  + '</blockquote>',
        'pre':   x => '<pre>'        + markup.escape(x.join('\n')) + '</pre>',
        'p':     x => '<p>'          + markup.inline(x.join('\n')) + '</p>',
        'h1':    x => '<h1>'         + x.map(markup.inline).join('</h1><h1>') + '</h1>',
        'h2':    x => '<h2>'         + x.map(markup.inline).join('</h2><h2>') + '</h2>',
        'h3':    x => '<h3>'         + x.map(markup.inline).join('</h3><h3>') + '</h3>',
        'ol':    x => '<ol><li>'     + x.map(markup.inline).join('</li><li>') + '</li></ol>',
        'ul':    x => '<ul><li>'     + x.map(markup.inline).join('</li><li>') + '</li></ul>',
    },

    inlineRe: [
        { 'code':    /(`+)(.+?)\1/g          },
        { 'bold':    /\*\*((?:\\?.)+?)\*\*/g },
        { 'italic':  /\*((?:\\?.)+?)\*/g     },
        { 'strike':  /~~((?:\\?.)+?)~~/g     },
        { 'spoiler': /%%((?:\\?.)+?)%%/g     },
        { 'mdash':   /--/g                   },
        { 'link':    /\b(([a-z][a-z0-9+\.-]*:(?:[,\.?]?[^\s(<>)"\',\.?%]|%[0-9a-f]{2}|\([^\s(<>)"\']+\))+))/g },
        { 'link':    /\[(.*?)\]\(((?:[^()]+|\(.*?\)|[^)])*)\)/g },
        { 'text':    /\\?([\w-]*[^\W_-]\s*|.)/g },
    ],

    inlineFn: {
        'code':    (m, a, b) => '<code>'                 + markup.inlineSafe(b) + '</code>',
        'bold':    (m, a)    => '<b>'                    + markup.inlineSafe(a) + '</b>',
        'italic':  (m, a)    => '<i>'                    + markup.inlineSafe(a) + '</i>',
        'strike':  (m, a)    => '<del>'                  + markup.inlineSafe(a) + '</del>',
        'spoiler': (m, a)    => '<span class="spoiler">' + markup.inlineSafe(a) + '</span>',
        'link':    (m, a, b) => `<a href="${b}" target="_blank">${a}</a>`,
        'mdash':   (m)       => '&mdash;',
        'text':    (m, a)    => a,
    },

    parse(text) {
        let key = 'break';
        let block = [];
        let result = '';
        for (let line of text.split('\n')) {
            for (let r of markup.blockRe) {
                let k; for (k in r) {}
                r[k].lastIndex = 0;
                if (!r[k].test(line))
                    continue;
                if (k !== key) {
                    result += markup.blockFn[key](block);
                    key = k;
                    block = [];
                }
                block.push(line.substr(r[k].lastIndex));
                break;
            }
        }
        return result + markup.blockFn[key](block);
    },

    escape: x =>
        x.replace(/[&<>"]/g, x => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'})[x]),

    inline: x =>
        markup.inlineSafe(markup.escape(x)),

    inlineSafe(x) {
        let result = '';
        while (x !== "") {
            let best = {key: 'text', start: x.length, end: x.length, groups: ['']};

            for (let i of markup.inlineRe) {
                let key; for (key in i) {}
                i[key].lastIndex = 0;
                let groups = i[key].exec(x);
                if (groups !== null && i[key].lastIndex - groups[0].length < best.start)
                    best = {key, start: i[key].lastIndex - groups[0].length, end: i[key].lastIndex, groups};
            }

            result += x.substr(0, best.start) + markup.inlineFn[best.key](...best.groups);
            x = x.substr(best.end);
        }
        return result;
    },
};


$init['[data-markup]'] = e => {
    let r = document.createElement('div');
    r.dataset.markup = 'html';
    r.innerHTML = markup.parse(e.textContent);
    new MutationObserver(() => {
        r.innerHTML = markup.parse(e.textContent);
    }).observe(e, {childList: true, characterData: true});
    e.parentElement.insertBefore(r, e);
};
