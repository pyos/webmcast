'use strict';

NodeList.prototype[Symbol.iterator] = Array.prototype[Symbol.iterator];


let $ = {
    rules: {},

    apply(e, rs) {
        for (let f in rs) if (rs.hasOwnProperty(f)) {
            if (e.matches && e.matches(f))
                rs[f](e);
            for (let c of e.querySelectorAll(f))
                rs[f](c);
        }
    },

    init: e => ($.apply(e, $.rules), e),

    extend: rs => ($.apply(document, rs), Object.assign($.rules, rs)),

    template: id =>
        $.init(document.importNode(document.getElementById(id).content, true)),
};


$.markup = {
    blocks: [
        { re: /^ {4}|^\t/g,            f: x => `<pre>${x.join('\n')}</pre>` },
        { re: /^\s*$/g,                f: x => '' },
        { re: /^\s*(?:[*-]\s*){3,}$/g, f: x => '<hr/>' },
        { re: /^\s*&gt;/g,             f: x => `<blockquote>${$.markup.block(x.join('\n'))}</blockquote>` },
        { re: /^\s*\d+\.\s+/g,         f: x => `<ol><li>${x.map($.markup.inline).join('</li><li>')}</li></ol>` },
        { re: /^\s*[*+-]\s+/g,         f: x => `<ul><li>${x.map($.markup.inline).join('</li><li>')}</li></ul>` },
        { re: /^\s*###\s*/g,           f: x => `<h3>${x.map($.markup.inline).join('</h3><h3>')}</h3>` },
        { re: /^\s*##\s*/g,            f: x => `<h2>${x.map($.markup.inline).join('</h2><h2>')}</h2>` },
        { re: /^\s*#\s*/g,             f: x => `<h1>${x.map($.markup.inline).join('</h1><h1>')}</h1>` },
        { re: /^\s*/g,                 f: x => `<p>${$.markup.inline(x.join('\n'))}</p>` },
    ],

    inlineRe: [
        { link:    /\b[a-z][a-z0-9+\.-]*:(?:[,\.?]?[^\s(<>)"\',\.?%]|%[0-9a-f]{2}|\([^\s(<>)"\']+\))+/g },
        { link:    /\[(.*?)\]\(((?:[^()]+|\(.*?\)|[^)])*)\)/g },
        { code:    /(`+)(.+?)\1/g               },
        { bold:    /\*\*((?:\\.|[^\\])+?)\*\*/g },
        { italic:  /\*((?:\\.|[^\\])+?)\*/g     },
        { strike:  /~~((?:\\.|[^\\])+?)~~/g     },
        { spoiler: /%%((?:\\.|[^\\])+?)%%/g     },
        { mdash:   /--/g                        },
        { esc:     /\\(.)/g                     },
    ],

    inlineFn: {
        link:    (m, a, b) => `<a href="${(b||m).replace(/"/g, '&quot;')}" target="_blank" rel="noopener noreferrer">${a||m}</a>`,
        code:    (m, a, b) => `<code>${$.markup.inline(b)}</code>`,
        bold:    (m, a)    => `<b>${$.markup.inline(a)}</b>`,
        italic:  (m, a)    => `<i>${$.markup.inline(a)}</i>`,
        strike:  (m, a)    => `<del>${$.markup.inline(a)}</del>`,
        spoiler: (m, a)    => `<x-spoiler>${$.markup.inline(a)}</x-spoiler>`,
        mdash:   (m)       => '&mdash;',
        esc:     (m, a)    => a,
    },

    block(text) {
        let last = $.markup.blocks[1], block = [], result = '';
        for (let line of text.split('\n')) for (let r of $.markup.blocks) {
            if (r.re.lastIndex = 0, r.re.test(line)) {
                if (r !== last)
                    result += last.f(block.splice(0, block.length));
                block.push(line.substr((last = r).re.lastIndex));
                break;
            }
        }
        return result + last.f(block);
    },

    inline(x) {
        for (let result = '';;) {
            let first = {start: x.length};
            for (let r of $.markup.inlineRe) for (let k in r) {
                r[k].lastIndex = 0;
                let groups = r[k].exec(x);
                if (groups && r[k].lastIndex - groups[0].length < first.start)
                    first = {k, start: r[k].lastIndex - groups[0].length, end: r[k].lastIndex, groups};
            }

            if (!first.k)
                return result + x;
            result += x.substr(0, first.start) + $.markup.inlineFn[first.k](...first.groups);
            x = x.substr(first.end);
        }
    },
};


$.form = {
    onDocumentReload(doc) {
        document.documentElement.replaceChild(doc.body, document.body);
        $.init(document.body);
        return true;
    },

    enable(e) {
        delete e.dataset.status;
        for (let i of e.querySelectorAll('[disabled="_"]'))
            i.disabled = false;
    },

    disable(e) {
        e.dataset.status = 'loading';
        for (let i of e.querySelectorAll(':enabled'))
            i.setAttribute('disabled', '_');
    },

    submit: (e) => new Promise((resolve, reject) => {
        let xhr = new XMLHttpRequest();
        xhr.onload = xhr.onerror = ev => {
            $.form.enable(e);
            (xhr.response && xhr.status < 300 ? resolve : reject)(xhr);
        };
        xhr.responseType = 'document';
        xhr.open(e.getAttribute('method') || e.dataset.method || 'GET', e.getAttribute('action') || e.dataset.action);
        xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
        xhr.send(new FormData(e));
        $.form.disable(e);
    }),
};


$.observeData = (e, attr, fallback, f) => {
    new MutationObserver(_ => f(e.dataset[attr])).observe(e, {attributes: true, attributeFilter: ['data-' + attr]});
    if (e.dataset[attr] || fallback)
        e.dataset[attr] = e.dataset[attr] || fallback;
};


$.delayedPair = (delay, f, g, t /* undefined */) =>
    (...args) => t = (clearTimeout(t), f(...args), setTimeout(() => g(...args), delay));


$._nativeScrollbarWidth = (() => {
    let b = document.body, e = document.createElement('div'), r;
    e.style.position = 'fixed';
    e.style.overflow = 'scroll';
    b.appendChild(e);
    r = e.offsetWidth - e.clientWidth;
    b.removeChild(e);
    return r;
})();


$._reflowAllColumns = () => {
    for (let e of document.querySelectorAll('x-columns'))
        e.dataset.columns = '';
};


$.extend({
    '[data-scrollbar]'(e) {
        if ($._nativeScrollbarWidth === 0 || e.style.marginRight !== '')
            return;  // native scrollbar is already floating

        let track = document.createElement('x-scrollbar');
        let thumb = document.createElement('x-slider');
        track.appendChild(thumb);

        let hide = () => { track.style.opacity = 0; };
        let show = $.delayedPair(1000, () => {
            let st = e.scrollTop, sh = e.scrollHeight, ch = e.clientHeight;
            if (st + ch >= sh) {  // the scrollbar may be keeping the element from collapsing
                track.style.transform = '';
                e.scrollTop = e.scrollHeight - ch;
            }
            if (sh > ch) {
                track.style.opacity   = 1;
                track.style.transform = `translateY(${st}px)`;
                thumb.style.transform = `translateY(${st / sh * 100 - 50}%) scaleY(${ch / sh}) translateY(50%)`;
            }
        }, hide);

        let innermost = ev => {
            for (let t = ev.target; t !== null; t = t.parentElement)
                if (t.hasAttribute('data-scrollbar') && t.scrollHeight > t.clientHeight)
                    return t;
        };

        e.style.overflowY   = 'scroll';
        e.style.marginRight = `${-$._nativeScrollbarWidth}px`;
        e.appendChild(track);
        e.addEventListener('scroll',     ev => window.requestAnimationFrame(show));
        e.addEventListener('mouseleave', ev => window.requestAnimationFrame(hide));
        e.addEventListener('mousemove',  ev => window.requestAnimationFrame(innermost(ev) === e ? show : hide));
        e.addEventListener('wheel',      ev => {
            let t = innermost(ev);
            if (t && ((ev.deltaY > 0 && t.scrollTop >= t.scrollHeight - t.clientHeight)
                   || (ev.deltaY < 0 && t.scrollTop === 0)))
                ev.preventDefault();
        });
    },

    '[data-remote-element]'(e) {
        $.form.submit(e).then(xhr => {
            try {
                for (let c of Array.from(xhr.response.querySelector(e.dataset.remoteElement).children))
                    e.appendChild($.init(c));
            } catch (err) {
                console.log('could not fetch remote element:', err);
                e.setAttribute('data-status', 'error');
            }
        }).catch(xhr => e.setAttribute('data-status', 'error'));
    },

    '[data-tabs]'(e) {
        let bar = document.createElement('x-tabbar');
        bar.addEventListener('click', ev =>
            e.dataset.tabs = ev.target.dataset.tab || e.dataset.tabs);

        new MutationObserver(_ => {
            bar.innerHTML = '';
            for (let tab of e.children) if (tab.dataset.tab) {
                let item = document.createElement('div');
                let head = tab.querySelector('[data-tab-title]');
                if (head)
                    item.innerHTML = head.innerHTML;
                else
                    item.textContent = tab.dataset.tab;
                item.dataset.tab = tab.dataset.tab;
                bar.appendChild(item);
            }
            if (bar.children.length)
                e.dataset.tabs = e.dataset.tabs || bar.children[0].dataset.tab;
        }).observe(e, {childList: true});

        $.observeData(e, 'tabs', '', active => {
            for (let tab of bar.children) if (tab.dataset.tab)
                tab.classList[tab.dataset.tab === active ? 'add' : 'remove']('active');
            for (let tab of e.children) if (tab.dataset.tab)
                tab[tab.dataset.tab === active ? 'removeAttribute' : 'setAttribute']('hidden', '');
        });

        if (e.children.length)
            e.insertBefore(bar, e.children[0]);
        else
            e.appendChild(bar);
    },

    '[data-markup]'(e) {
        let r = document.createElement('div');
        r.dataset.markup = 'html';
        r.innerHTML = $.markup.block(e.innerHTML);
        new MutationObserver(_ => r.innerHTML = $.markup.block(e.innerHTML)).observe(e, {childList: true, characterData: true});
        e.parentElement.insertBefore(r, e);
    },

    'x-columns'(e) {
        let reflow = () => {
            mut.disconnect();
            // keep total size contant while inner elements are being repositioned
            // to prevent the scroll from jumping up a little bit.
            e.style.height = `${e.offsetHeight}px`;
            let cols = Array.from(e.children);
            let cells = [].concat(...cols.map(c => Array.from(c.children)));
            for (let c of cols)
                c.innerHTML = '';
            for (let c of cells.sort((x, y) => x.dataset.order - y.dataset.order)) {
                let k = 0;
                for (let i = 1; i < cols.length; i++)
                    if (cols[i].offsetTop + cols[i].offsetHeight < cols[k].offsetTop + cols[k].offsetHeight)
                        k = i;
                cols[k].appendChild(c);
            }
            e.style.height = '';
            mut.observe(e, opt);
        };
        let mut = new MutationObserver(reflow);
        let opt = {attributes: true, childList: true, characterData: true, subtree: true};
        reflow();
        window.addEventListener('resize', $._reflowAllColumns);
    },

    'x-modal'(e) {
        let outer = document.createElement('x-modal-cover');
        let inner = document.createElement('x-modal-bg');
        let close = document.createElement('a');
        e.parentNode.appendChild(outer);
        close.setAttribute('href', '#');
        close.classList.add('button');
        close.classList.add('close');
        close.classList.add('icon');
        close.addEventListener('click', (ev) => (ev.preventDefault(), outer.remove()));
        outer.addEventListener('click', (ev) => ev.target === ev.currentTarget ? outer.remove() : 1);
        outer.appendChild(inner);
        inner.appendChild(e);
        inner.appendChild(close);
    },

    'body'(e) {
        e.addEventListener('focusin', ev => {
            let modals = e.querySelectorAll('x-modal-cover');
            if (modals.length === 0)
                return;
            for (let t = ev.target; t !== null; t = t.parentElement)
                if (t === modals[modals.length - 1])
                    return;
            ev.target.blur();  // focus must not escape modal dialogs.
        });
    },

    'a[href="/user/new"], a[href="/user/login"], a[href="/user/restore"]'(e) {
        let tab = e.getAttribute('href');
        e.addEventListener('click', ev => {
            ev.preventDefault();
            for (let p = e.parentElement; p !== null; p = p.parentElement)
                if (p.hasAttribute('data-tabs'))
                    return (p.dataset.tabs = tab);

            let it = $.template('login-form-template');
            it.querySelector('[data-tabs]').dataset.tabs = tab;
            document.body.appendChild(it);
        });
    },

    'form'(e) {
        let submit = ev =>
            (ev.preventDefault(), e.dispatchEvent(new Event('submit', {cancelable: true})));
        let submitOnReturn = ev =>
            ev.keyCode === 13 && !ev.shiftKey ? submit(ev) : true;
        for (let c of e.querySelectorAll('a[data-submit]'))
            c.addEventListener('click', submit);
        for (let c of e.querySelectorAll('input[type="checkbox"][data-submit]'))
            c.addEventListener('change', submit);
        for (let c of e.querySelectorAll('textarea[data-submit]'))
            c.addEventListener('keydown', submitOnReturn);

        if ((e.getAttribute('method') || '').toLowerCase() === 'post' && !('noXhr' in e.dataset))
            e.addEventListener('submit', ev => {
                ev.preventDefault();
                $.form.submit(e).then(xhr => {
                    try {
                        $.form.disable(e);
                        if (xhr.responseURL === location.href && $.form.onDocumentReload(xhr.response))
                            return $.form.enable(e);
                    } catch (err) {
                        console.log('Error in onDocumentReload:', err);
                    }
                    location.href = xhr.responseURL;
                }).catch(xhr => {
                    e.classList.add('error');
                    e.querySelector('.error').textContent =
                        xhr.response ? xhr.response.getElementById('message').textContent
                                     : 'Could not connect to server.';
                });
            });
    },

    'x-range'(e) {
        let step = isNaN(+e.dataset.step) ? 0.05 : +e.dataset.step;
        let slider = document.createElement('x-slider');
        $.observeData(e, 'value', 1, v => slider.style.width = `${+v * 100}%`);
        e.appendChild(slider);

        let change = x =>
            e.dispatchEvent(new CustomEvent('change', {detail: Math.min(1, Math.max(0, x))}));

        let select = ev => {
            ev.preventDefault();
            let r = e.getBoundingClientRect();
            change(((ev.touches || [ev])[0].clientX - r.left) / (r.right - r.left));
        };

        e.addEventListener('mousedown',  select);
        e.addEventListener('touchstart', select);
        e.addEventListener('touchmove',  select);
        e.addEventListener('mousedown',  _ => e.addEventListener('mousemove', select));
        e.addEventListener('mouseup',    _ => e.removeEventListener('mousemove', select));
        e.addEventListener('mouseleave', _ => e.removeEventListener('mousemove', select));
        e.addEventListener('keydown', ev => {
            if (ev.keyCode === 37) change(+e.dataset.value - step);
            if (ev.keyCode === 39) change(+e.dataset.value + step);
        });
    },

    'span.hostname'(e) {
        e.textContent = location.protocol + '//' + location.host;
    },
});
