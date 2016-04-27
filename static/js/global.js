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


let init = {
    all: (e) => {
        for (let f in init) if (init.hasOwnProperty(f) && f != 'all')
            for (let c of e.querySelectorAll(f))
                init[f](c);
        return e;
    },

    '[data-scrollbar]': (e) => {
        if (nativeScrollbarWidth === 0 || e.style.marginRight !== '')
            return;  // native scrollbar is already floating

        let timer = null;
        let track = document.createElement('div');
        let thumb = document.createElement('div');
        thumb.classList.add('thumb');
        track.classList.add('track');
        track.classList.add('hide');
        track.appendChild(thumb);

        let show = () => {
            thumb.style.height =  `${e.clientHeight / e.scrollHeight * track.clientHeight}px`;
            thumb.style.top    =  `${e.scrollTop    / e.scrollHeight * track.clientHeight}px`;
            track.style.top    =  `${e.scrollTop}px`;
            track.style.bottom = `-${e.scrollTop}px`;
            if (e.scrollHeight > e.clientHeight) {
                window.clearTimeout(timer);
                track.classList.remove('hide');
                timer = window.setTimeout(() => track.classList.add('hide'), 1000);
            }
        };

        e.style.overflowY   = 'scroll';
        e.style.marginRight = `${-nativeScrollbarWidth}px`;
        e.appendChild(track);
        e.addEventListener('scroll',    show);
        e.addEventListener('mousemove', show);
        show();
    },

    '[data-tabs]': (e) => {
        let bar = document.createElement('div');
        bar.classList.add('tabbar');
        bar.addEventListener('click', (ev) =>
            e.setAttribute('data-tabs', ev.target.getAttribute('data-tab') || e.getAttribute('data-tabs')));

        let tabs = {};
        for (let tab of e.querySelectorAll('[data-tab]')) {
            if (tab.parentElement.parentElement !== e)
                continue;
            tabs[tab.getAttribute('data-tab')] = {t: tab.parentElement, b: tab};
            bar.appendChild(tab);
        }

        new MutationObserver(() => {
            for (let id in tabs) {
                tabs[id].b.classList.remove('active');
                tabs[id].t.setAttribute('hidden', '');
            }
            let active = e.getAttribute('data-tabs');
            tabs[active].b.classList.add('active');
            tabs[active].t.removeAttribute('hidden');
        }).observe(e, {attributes: true, attributeFilter: ['data-tabs']});

        e.insertBefore(bar, e.childNodes[0]);
        e.setAttribute('data-tabs', e.getAttribute('data-tabs'));
    },

    '[data-modal]': (e) => {
        let parent = e.parentNode;
        let outer  = document.createElement('div');
        let inner  = document.createElement('div');
        let scroll = document.createElement('div');
        let close  = document.createElement('a');
        outer.classList.add('modal-bg');
        inner.classList.add('modal');
        close.classList.add('button');
        close.classList.add('close');
        scroll.setAttribute('data-scrollbar', '');
        scroll.appendChild(e);
        parent.appendChild(outer);
        inner.appendChild(scroll);
        inner.appendChild(close);
        outer.appendChild(inner);
        outer.addEventListener('click', (e) => e.target === e.currentTarget ? outer.remove() : 1);
        close.addEventListener('click', (e) => outer.remove());
        // if the [data-scrollbar] initializer has already run, this element would be left
        // uninitialized. good thing that particular initializer is idempotent...
        init['[data-scrollbar]'](scroll);
    },

    'body': (e) => {
        e.addEventListener('focusin', (ev) => {
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

    'a[href^="/user/"]': (e) => {
        let tab = {
            '/user/new':     'signup',
            '/user/login':   'login',
            '/user/restore': 'restore',
        }[e.getAttribute('href')];

        if (tab) e.addEventListener('click', (ev) => {
            ev.preventDefault();
            for (let p = e.parentElement; p !== null; p = p.parentElement)
                if (p.hasAttribute('data-tabs'))
                    return p.setAttribute('data-tabs', tab);

            let it = document.importNode(document.querySelector('#login-form').content, true);
            it.firstElementChild.setAttribute('data-tabs', tab);
            document.body.appendChild(init.all(it));
        });
    },

    'form[data-xhrable]': (e) => {
        e.addEventListener('submit', (ev) => {
            ev.preventDefault();
            form.submit(e).then((xhr) => {
                if (xhr.status === 204)
                    window.location.reload();
                else
                    window.location = xhr.responseURL;
            }).catch((xhr, isNetworkError) => {
                e.classList.add('error');
                e.querySelector('.error').textContent =
                    isNetworkError ? 'Could not connect to server.'
                                   : xhr.response.getElementById('message').textContent;
            });
        });
    },
};


let form = {
    enable: (e) => {
        e.removeAttribute('data-status');
        for (let input of e.querySelectorAll(':enabled'))
            input.removeAttribute('disabled', '');
    },

    disable: (e) => {
        e.setAttribute('data-status', 'loading');
        e.setAttribute('data-status-with-bg', '');
        for (let input of e.querySelectorAll(':disabled'))
            input.setAttribute('disabled', '');
    },

    submit: (e) => new Promise((resolve, reject) => {
        let xhr = new XMLHttpRequest();

        xhr.onload = (ev) => {
            form.enable(e);
            if (xhr.status >= 400)
                reject(xhr, false);
            else
                resolve(xhr);
        };

        xhr.onerror = (ev) => {
            form.enable(e);
            reject(xhr, true);
        };

        xhr.responseType = 'document';
        xhr.open(e.getAttribute('method'), e.getAttribute('action'));
        xhr.setRequestHeader('X-Requested-With', 'XMLHttpRequest');
        xhr.send(new FormData(e));
        form.disable(e);
    }),
};


document.addEventListener('DOMContentLoaded', () => init.all(document));
