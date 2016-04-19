'use strict';


let nativeScrollbarWidth = (() => {
    let e = document.createElement('div');
    e.style.position = 'absolute';
    e.style.top      = '-200px';
    e.style.width    = '100px';
    e.style.height   = '100px';
    e.style.overflow = 'scroll';
    document.body.appendChild(e);
    let r = e.offsetWidth - e.clientWidth;
    e.remove();
    return r;
})();


let initElement = (e) => {
    for (let es = e.querySelectorAll('[data-scrollable]'), i = 0; i < es.length; i++)
        createCustomScrollbar(es[i]);
    for (let es = e.querySelectorAll('[data-tabs]'), i = 0; i < es.length; i++)
        createTabSelector(es[i]);
    return e;
};


let createCustomScrollbar = (e) => {
    if (nativeScrollbarWidth === 0)
        return;

    e.style.overflowY = 'hidden';
    let track = document.createElement('div');
    let thumb = document.createElement('div');
    thumb.classList.add('thumb');
    track.classList.add('scrollbar');
    track.classList.add('hidden');
    track.appendChild(thumb);

    let timeout = null;
    let show = () => {
        thumb.style.top          =  `${e.scrollTop    / e.scrollHeight * track.clientHeight}px`;
        thumb.style.height       =  `${e.clientHeight / e.scrollHeight * track.clientHeight}px`;
        track.style.marginTop    =  `${e.scrollTop}px`;
        track.style.marginBottom = `-${e.scrollTop}px`;
        if (e.scrollHeight > e.clientHeight) {
            if (timeout !== null)
                window.clearTimeout(timeout);
            track.classList.remove('hidden');
            timeout = window.setTimeout(() => track.classList.add('hidden'), 1000);
            e.style.overflowY   = 'scroll';
            e.style.marginRight = `-${nativeScrollbarWidth}px`;
        } else {
            timeout = null;
            e.style.overflowY   = 'hidden';
            e.style.marginRight = '0';
        }
    };

    e.appendChild(track);
    e.addEventListener('scroll',    show);
    e.addEventListener('mousemove', show);
};


let createTabSelector = (e) => {
    let init = null;
    let tabs = {};

    let onSelect = () => {
        let active = e.getAttribute('data-tabs') || init;
        for (let id in tabs) {
            if (id === active) {
                tabs[id].elem.removeAttribute('hidden');
                tabs[id].tab.classList.add('active');
            } else {
                tabs[id].elem.setAttribute('hidden', '1');
                tabs[id].tab.classList.remove('active');
            }
        }
    };

    let setThisActive = function () {
        e.setAttribute('data-tabs', this.getAttribute('data-tab'));
    };

    let bar = document.createElement('div');
    bar.classList.add('tabbar');

    let children = e.querySelectorAll('[data-tab]');
    for (let i = 0; i < children.length; i++) {
        let c = children[i], id = c.getAttribute('data-tab');
        if (c.parentElement !== null && c.parentElement.parentElement === e) {
            if (init === null)
                init = id;
            tabs[id] = {tab: c, elem: c.parentElement};
            c.parentElement.removeChild(c);
            c.addEventListener('click', setThisActive);
            bar.appendChild(c);
        }
    }

    e.insertBefore(bar, e.childNodes[0]);

    onSelect();
    return new MutationObserver(onSelect).observe(e,
        {attributes: true, attributeFilter: ['data-tabs']});
};


let showModal = (e) => {
    let close  = document.createElement('a');
    let outer  = document.createElement('div');
    let inner  = document.createElement('div');
    let scroll = document.createElement('div');
    outer.classList.add('modal-outer');
    inner.classList.add('modal-inner');
    close.classList.add('button');
    close.classList.add('close');
    scroll.setAttribute('data-scrollable', '1');
    scroll.appendChild(e);
    inner.appendChild(scroll);
    inner.appendChild(close);
    outer.appendChild(inner);
    document.body.appendChild(initElement(outer));

    outer.addEventListener('click', (e) => {
        if (e.target === e.currentTarget || e.target === close)
            outer.remove();
    });

    return e;
};


let showLoginForm = (signup) => {
    let template = document.querySelector('template#login-form');
    if (template === null)
        return;

    let submitForm = function () {
        // TODO ajax
    };

    let it = document.importNode(template.content, true).firstElementChild;
    if (signup)
        it.setAttribute('data-tabs', 'signup');

    it.querySelector('.go-to-restore').addEventListener('click', () =>
        it.setAttribute('data-tabs', 'restore'));

    let forms = it.querySelector('form');
    for (let i = 0; i < forms.length; i++)
        forms[i].addEventListener('submit', submitForm);
    return showModal(it);
};


initElement(document);

document.querySelector('nav .login').addEventListener('click', () => showLoginForm(false));
document.querySelector('nav .signup').addEventListener('click', () => showLoginForm(true));
