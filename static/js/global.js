'use strict';
// dammit, chrome, you were supposed to have the best standards support...
NodeList.prototype[Symbol.iterator] = Array.prototype[Symbol.iterator];


const nativeScrollbarWidth = (() => {
    let e = document.createElement('div');
    e.style.position = 'fixed';
    e.style.width    = '100px';
    e.style.height   = '100px';
    e.style.overflow = 'scroll';
    document.body.appendChild(e);
    let r = e.offsetWidth - e.clientWidth;
    document.body.removeChild(e);
    return r;
})();


const _reflowAllColumns = () => {
    for (let e of document.querySelectorAll('[data-columns]'))
        e.dataset.columns = '';
};


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
        { 'code':    /(`+)(.+?)\1/g               },
        { 'bold':    /\*\*((?:\\.|[^\\])+?)\*\*/g },
        { 'italic':  /\*((?:\\.|[^\\])+?)\*/g     },
        { 'strike':  /~~((?:\\.|[^\\])+?)~~/g     },
        { 'spoiler': /%%((?:\\.|[^\\])+?)%%/g     },
        { 'mdash':   /--/g                        },
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
        'link':    (m, a, b) => `<a href="${b}" ${b !== a ? `title="${b}"` : ''} target="_blank" rel="noopener noreferrer">${a}</a>`,
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


let $init = {
    all(e) {
        let isElem = e instanceof Element;
        for (let f in $init) {
            if ($init.hasOwnProperty(f) && f != 'all' && f !== 'template') {
                if (isElem && e.matches(f))
                    $init[f](e);
                for (let c of e.querySelectorAll(f))
                    $init[f](c);
            }
        }
        return e;
    },

    template(id) {
        return $init.all(document.importNode(document.getElementById(id).content, true));
    },

    '[data-scrollbar]'(e) {
        if (nativeScrollbarWidth === 0 || e.style.marginRight !== '')
            return;  // native scrollbar is already floating

        let timer = null;
        let track = document.createElement('div');
        let thumb = document.createElement('div');
        thumb.classList.add('thumb');
        track.classList.add('track');
        track.appendChild(thumb);

        let hide = () => { track.style.opacity = 0; };
        let show = () => {
            if (e.scrollHeight > e.clientHeight) {
                window.clearTimeout(timer);
                let h = e.clientHeight / e.scrollHeight;
                track.style.opacity   = 1;
                track.style.transform = `translateY(${e.scrollTop}px)`;
                thumb.style.transform = `translateY(${e.scrollTop / e.scrollHeight * 100 + h * 50 - 50}%) scaleY(${h})`;
                timer = window.setTimeout(hide, 1000);
            }
        };

        let innermost = ev => {
            for (let t = ev.target; t !== null; t = t.parentElement)
                if (t.hasAttribute('data-scrollbar') && t.scrollHeight > t.clientHeight)
                    return t;
        };

        let trigger = ev =>
            window.requestAnimationFrame(innermost(ev) === e ? show : hide);

        let ignoreOvershoot = ev => {
            let t = innermost(ev);
            if (t && ((ev.deltaY > 0 && t.scrollTop >= t.scrollHeight - t.clientHeight)
                   || (ev.deltaY < 0 && t.scrollTop === 0)))
                ev.preventDefault();
        };

        e.style.overflowY   = 'scroll';
        e.style.marginRight = `${-nativeScrollbarWidth}px`;
        e.appendChild(track);
        e.addEventListener('mouseleave', _ => window.requestAnimationFrame(hide));
        e.addEventListener('mousemove',  trigger);
        e.addEventListener('scroll',     trigger);
        e.addEventListener('wheel',      ignoreOvershoot);
    },

    '[data-columns]'(e) {
        let cb;
        let opt = {attributes: true, childList: true, characterData: true, subtree: true};
        let mut = new MutationObserver(cb = () => {
            mut.disconnect();
            // keep total size contant while inner elements are being repositioned
            // to prevent the scroll from jumping up a little bit.
            e.style.height = `${e.offsetHeight}px`;
            let cols = Array.from(e.children);
            let cells = [];
            for (let c of cols)
                cells.splice(cells.length, 0, ...Array.from(c.children));
            cells.sort((x, y) => parseInt(x.dataset.order) - parseInt(y.dataset.order));
            for (let c of cols)
                c.innerHTML = '';
            for (let c of cells) {
                let k = 0;
                for (let i = 1; i < cols.length; i++)
                    if (cols[i].offsetTop + cols[i].offsetHeight < cols[k].offsetTop + cols[k].offsetHeight)
                        k = i;
                cols[k].appendChild(c);
            }
            e.style.height = '';
            mut.observe(e, opt);
        });
        cb();
        window.addEventListener('resize', _reflowAllColumns);
    },

    '[data-tabs]'(e) {
        let bar = document.createElement('div');
        bar.classList.add('tabbar');
        bar.addEventListener('click', ev =>
            e.dataset.tabs = ev.target.dataset.tab || e.dataset.tabs);

        let tabs = {};
        let init = e.dataset.tabs;
        for (let tab of e.querySelectorAll('[data-tab]')) {
            init = init || tab.dataset.tab;
            if (tab.dataset.tabTitle) {
                let item = document.createElement('span');
                item.dataset.tab = tab.dataset.tab;
                item.textContent = tab.dataset.tabTitle;
                tabs[tab.dataset.tab] = {t: tab, b: item};
                bar.appendChild(item);
            } else if (tab.parentElement.parentElement === e) {
                tabs[tab.dataset.tab] = {t: tab.parentElement, b: tab};
                bar.appendChild(tab);
            }
        }

        new MutationObserver(_ => {
            for (let id in tabs) {
                tabs[id].b.classList.remove('active');
                tabs[id].t.setAttribute('hidden', '');
            }
            let active = e.dataset.tabs;
            tabs[active].b.classList.add('active');
            tabs[active].t.removeAttribute('hidden');
        }).observe(e, {attributes: true, attributeFilter: ['data-tabs']});

        e.insertBefore(bar, e.childNodes[0]);
        e.dataset.tabs = init;
    },

    '[data-markup]'(e) {
        let r = document.createElement('div');
        r.dataset.markup = 'html';
        r.innerHTML = markup.parse(e.textContent);
        new MutationObserver(() => {
            r.innerHTML = markup.parse(e.textContent);
        }).observe(e, {childList: true, characterData: true});
        e.parentElement.insertBefore(r, e);
    },

    '[data-modal]'(e) {
        let parent = e.parentNode;
        let outer  = document.createElement('div');
        let inner  = document.createElement('div');
        let scroll = document.createElement('div');
        let close  = document.createElement('a');
        outer.classList.add('modal-bg');
        inner.classList.add('modal');
        close.classList.add('button');
        close.classList.add('close');
        close.classList.add('icon');
        close.setAttribute('href', '#');
        scroll.setAttribute('data-scrollbar', '');
        scroll.appendChild(e);
        parent.appendChild(outer);
        inner.appendChild(scroll);
        inner.appendChild(close);
        outer.appendChild(inner);
        outer.addEventListener('click', (ev) => ev.target === ev.currentTarget ? outer.remove() : 1);
        close.addEventListener('click', (ev) => (ev.preventDefault(), outer.remove()));
        // if the [data-scrollbar] initializer has already run, this element would be left
        // uninitialized. good thing that particular initializer is idempotent...
        $init['[data-scrollbar]'](scroll);
    },

    'body'(e) {
        e.addEventListener('focusin', ev => {
            // focus must not escape modal dialogs.
            let modals = e.querySelectorAll('.modal-bg');
            if (modals.length === 0)
                return;
            for (let t = ev.target; t !== null; t = t.parentElement)
                if (t === modals[modals.length - 1])
                    return;
            ev.target.blur();
        });
    },

    'a[href^="/user/"]'(e) {
        let tab = {
            '/user/new':     'signup',
            '/user/login':   'login',
            '/user/restore': 'restore',
        }[e.getAttribute('href')];

        if (tab) e.addEventListener('click', ev => {
            ev.preventDefault();
            for (let p = e.parentElement; p !== null; p = p.parentElement)
                if (p.hasAttribute('data-tabs'))
                    return p.setAttribute('data-tabs', tab);

            let it = $init.template('login-form-template');
            it.querySelector('[data-tabs]').dataset.tabs = tab;
            document.body.appendChild(it);
        });
    },

    'form[data-xhrable]'(e) {
        e.addEventListener('submit', ev => {
            ev.preventDefault();
            $form.submit(e)
                 .then  (xhr => $form.onXHRSuccess(xhr, e))
                 .catch (err => $form.onXHRError(err, e));
        });
    },
};


let $form = {
    onDocumentReload(doc) {
        document.documentElement.replaceChild(doc.body, document.body);
        $init.all(document.body);
        return true;
    },

    onXHRSuccess(xhr, form) {
        try {
            $form.disable(form);
            if (xhr.responseURL === location.href && $form.onDocumentReload(xhr.response))
                return $form.enable(form);
            // TODO replace the whole `body`, like InstantClick does?
            //      gotta use `history.pushState` then, too...
        } catch (e) {
            console.log('failed to async-reload document:', e);
        }
        location.href = xhr.responseURL;
    },

    onXHRError(xhr, form) {
        form.classList.add('error');
        form.querySelector('.error').textContent =
            xhr.response ? xhr.response.getElementById('message').textContent
                         : 'Could not connect to server.';
    },

    enable(e) {
        delete e.dataset.status;
        for (let input of e.querySelectorAll('[disabled="by-$form"]'))
            input.removeAttribute('disabled', '');
    },

    disable(e) {
        e.dataset.status = 'loading';
        for (let input of e.querySelectorAll(':enabled'))
            input.setAttribute('disabled', 'by-$form');
    },

    submit: (e) => new Promise((resolve, reject) => {
        let xhr = new XMLHttpRequest();

        xhr.onload = ev => {
            $form.enable(e);
            if (xhr.status >= 300)
                reject(xhr);
            else
                resolve(xhr);
        };

        xhr.onerror = ev => {
            $form.enable(e);
            reject(xhr);
        };

        xhr.responseType = 'document';
        xhr.open(e.getAttribute('method'), e.getAttribute('action'));
        xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
        xhr.send(new FormData(e));
        $form.disable(e);
    }),
};


document.addEventListener('DOMContentLoaded', _ => $init.all(document));
