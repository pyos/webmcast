'use strict';


let initDocument = (d) => {
    Array.from(d.querySelectorAll('[data-scrollbar]')).map(attachFloatingScrollbar);
    Array.from(d.querySelectorAll('[data-tabs]')).map(attachTabBar);
    return d;
};


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


let attachFloatingScrollbar = (e) => {
    if (nativeScrollbarWidth === 0)
        return;  // native scrollbar is already floating

    let track = document.createElement('div');
    let thumb = document.createElement('div');
    thumb.classList.add('thumb');
    track.classList.add('track');
    track.classList.add('hide');
    track.appendChild(thumb);

    let timeout = null;
    let show = () => {
        thumb.style.height =  `${e.clientHeight / e.scrollHeight * track.clientHeight}px`;
        thumb.style.top    =  `${e.scrollTop    / e.scrollHeight * track.clientHeight}px`;
        track.style.top    =  `${e.scrollTop}px`;
        track.style.bottom = `-${e.scrollTop}px`;
        if (e.scrollHeight > e.clientHeight) {
            if (timeout !== null)
                window.clearTimeout(timeout);
            timeout = window.setTimeout(() => track.classList.add('hide'), 1000);
            track.classList.remove('hide');
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
    show();
};


let attachTabBar = (e) => {
    let bar  = document.createElement('div');
    let tabs = {};
    let btns = {};
    bar.classList.add('tabbar');

    let onSelect = () => {
        let active = e.getAttribute('data-tabs');
        for (let id in tabs) {
            if (id === active) {
                btns[id].classList.add('active');
                tabs[id].removeAttribute('hidden');
            } else {
                btns[id].classList.remove('active');
                tabs[id].setAttribute('hidden', '1');
            }
        }
    };

    let setThisActive = function () {
        e.setAttribute('data-tabs', this.getAttribute('data-tab'));
    };

    for (let tab of Array.from(e.querySelectorAll('[data-tab]'))) {
        if (tab.parentElement.parentElement !== e)
            continue;
        tab.addEventListener('click', setThisActive);
        tabs[tab.getAttribute('data-tab')] = tab.parentElement;
        btns[tab.getAttribute('data-tab')] = tab;
        bar.appendChild(tab);
    }

    new MutationObserver(onSelect).observe(e, {attributes: true, attributeFilter: ['data-tabs']});
    e.insertBefore(bar, e.childNodes[0]);
    onSelect();
};


let showModal = (e) => {
    let outer  = document.createElement('div');
    let inner  = document.createElement('div');
    let scroll = document.createElement('div');
    let close  = document.createElement('a');
    outer.classList.add('modal-bg');
    inner.classList.add('modal');
    close.classList.add('button');
    close.classList.add('close');
    scroll.setAttribute('data-scrollbar', '1');
    scroll.appendChild(e);
    inner.appendChild(scroll);
    inner.appendChild(close);
    outer.appendChild(inner);

    let onFocusChange = (ev) => {
        for (let t = ev.target; t !== null; t = t.parentElement)
            if (t === outer)
                return;
        ev.target.blur();  // FIXME should somehow set focus to the dialog instead
    };

    let onClose = () => {
        outer.remove();
        document.body.removeEventListener('focusin', onFocusChange);
    };

    document.body.appendChild(initDocument(outer));
    document.body.addEventListener('focusin', onFocusChange);
    outer.addEventListener('click', (e) =>
        e.target === e.currentTarget || e.target === close ? onClose() : null);
    return onClose;
};


let showLoginForm = (navbar, showSignup) => {
    let template = document.querySelector('template#login-form');
    if (template === null)
        return;

    let submitForm = function () {
        // TODO ajax
    };

    let it = initDocument(document.importNode(template.content, true)).firstElementChild;
    if (showSignup)
        it.setAttribute('data-tabs', 'signup');

    it.querySelector('.go-to-restore').addEventListener('click', (ev) => {
        ev.preventDefault();
        it.setAttribute('data-tabs', 'restore');
    });

    let forms = it.querySelector('form');
    for (let i = 0; i < forms.length; i++)
        forms[i].addEventListener('submit', submitForm);
    return showModal(it);
};


let initNavBar = (e) => {
    let onUserSwap = () => {
        let user = e.getAttribute('data-user');
        if (user !== null) {
            e.querySelector('.username').textContent = user;
        }
    };

    new MutationObserver(onUserSwap).observe(e,
        {attributes: true, attributeFilter: ['data-user']});
    onUserSwap();

    e.querySelector('.login').addEventListener('click', (ev) => {
        ev.preventDefault();
        showLoginForm(e, false);
    });

    e.querySelector('.signup').addEventListener('click', (ev) => {
        ev.preventDefault();
        showLoginForm(e, true);
    });
};


initDocument(document);
initNavBar(document.querySelector('nav'));
