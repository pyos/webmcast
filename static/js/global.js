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

        let trigger = ev => {
            for (let t = ev.target; t !== null; t = t.parentElement)
                if (t.hasAttribute('data-scrollbar'))
                    return window.requestAnimationFrame(t === e ? show : hide);
        };

        e.style.overflowY   = 'scroll';
        e.style.marginRight = `${-nativeScrollbarWidth}px`;
        e.appendChild(track);
        e.addEventListener('mouseleave', _ => window.requestAnimationFrame(hide));
        e.addEventListener('mousemove',  trigger);
        e.addEventListener('scroll',     trigger);
    },

    '[data-tabs]'(e) {
        let bar = document.createElement('div');
        bar.classList.add('tabbar');
        bar.addEventListener('click', ev =>
            e.dataset.tabs = ev.target.dataset.tab || e.dataset.tabs);

        let tabs = {};
        for (let tab of e.querySelectorAll('[data-tab]')) {
            if (tab.parentElement.parentElement !== e)
                continue;
            tabs[tab.dataset.tab] = {t: tab.parentElement, b: tab};
            bar.appendChild(tab);
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
        e.dataset.tabs = e.dataset.tabs;
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
    onDocumentReload(body) {
        return false;  // override & return `true` if preserving some state is desired
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
