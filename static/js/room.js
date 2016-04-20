'use strict'; /* global screenfull */

if (screenfull.enabled) {
    document.addEventListener(screenfull.raw.fullscreenchange, () => {
        if (screenfull.isFullscreen)
            screenfull.element.classList.add('is-fullscreen');
        else
            document.querySelector('.is-fullscreen').classList.remove('is-fullscreen');
    });

    document.addEventListener(screenfull.raw.fullscreenerror, () =>
        document.body.classList.add('no-fullscreen'));
} else
    document.body.classList.add('no-fullscreen');


let RPC = function(url, ...objects) {
    this.nextID   = 0;
    this.events   = {};
    this.requests = {};
    this.socket   = new WebSocket(url);

    this.socket.onopen = () => {
        for (let object of objects)
            object.onLoad(this);
    };

    this.socket.onclose = (ev) => {
        for (let object of objects)
            object.onUnload();
    };

    this.socket.onmessage = (ev) => {
        let msg = JSON.parse(ev.data);

        if (msg.id === undefined)
            if (msg.method in this.events)
                this.events[msg.method](...msg.params);

        if (msg.id in this.requests) {
            let cb = this.requests[msg.id];
            delete this.requests[msg.id];
            if (msg.error === undefined)
                cb.resolve(msg.result);
            else
                cb.reject(msg.error);
        }
    };
};


RPC.prototype.send = function (method, ...params) {
    return new Promise((resolve, reject) => {
        let id = this.nextID++ & 0xFFFF;
        this.socket.send(JSON.stringify({ jsonrpc: '2.0', id, method, params }));
        this.requests[id] = { resolve, reject };
    });
};


RPC.prototype.connect = function (event, cb) {
    if (cb === undefined)
        delete this.events[event];
    else
        this.events[event] = cb;
};


let View = function (root, info, stream) {
    let rpc    = null;
    let view   = root.querySelector('video');
    let status = root.querySelector('.status');
    let volume = root.querySelector('.volume');

    let onVolumeSelect = (e) => {
        e.preventDefault();
        let r = volume.getBoundingClientRect();
        let x = Math.min(r.right, Math.max(r.left, e.touches ? e.touches[0].clientX : e.clientX));
        view.volume = (x - r.left) / (r.right - r.left);
        view.muted  = false;
    };

    let onVolumeChange = (v, muted) => {
        let e = volume.querySelector('.slider');
        let r = volume.getBoundingClientRect();
        e.style.left = `${v * (r.right - r.left)}px`;
        e.style.top = `${(1 - v) * (r.bottom - r.top)}px`;
        if (muted)
            root.classList.add('muted');
        else
            root.classList.remove('muted');
    };

    let onTimeUpdate = (t) => {
        // let leftPad = require('left-pad');
        status.textContent = `${(t / 60)|0}:${t % 60 < 10 ? '0' : ''}${(t|0) % 60}`;
    };

    let onDone = () => {
        root.setAttribute('data-status',
            view.error === null || view.error.code === 4 ? 'ended' : 'error');
        status.textContent = view.error === null   ? 'stream ended'
                           : view.error.code === 1 ? 'aborted'
                           : view.error.code === 2 ? 'network error'
                           : view.error.code === 3 ? 'decoding error'
                           : /* view.error.code === 4 ? */ 'stream ended';
    };

    let onLoad = () => {
        root.setAttribute('data-status', 'loading');
        status.textContent = 'loading';
    };

    let onPlay = () => {
        root.setAttribute('data-status', 'playing');
        status.textContent = 'playing';
    };

    let hideCursorTimeout = null;
    let hideCursorLater = () => {
        showCursor();
        hideCursorTimeout = window.setTimeout(() => {
            hideCursorTimeout = null;
            document.body.classList.add('hide-cursor');
        }, 3000);
    };

    let showCursor = () => {
        if (hideCursorTimeout !== null)
            window.clearTimeout(hideCursorTimeout);
        else
            document.body.classList.remove('hide-cursor');
        hideCursorTimeout = null;
    };

    view.addEventListener('loadstart',      onLoad);
    view.addEventListener('loadedmetadata', onPlay);
    view.addEventListener('error',          onDone);
    view.addEventListener('ended',          onDone);
    view.addEventListener('timeupdate', () => onTimeUpdate(view.currentTime));
    view.addEventListener('volumechange', () => onVolumeChange(view.volume, view.muted));
    // TODO playing, waiting, stalled (not sure whether these events are actually emitted)

    view.addEventListener('mouseenter', hideCursorLater);
    view.addEventListener('mouseleave', showCursor);
    view.addEventListener('mouseenter', () =>
        view.addEventListener('mousemove', hideCursorLater));
    view.addEventListener('mouseleave', () =>
        view.removeEventListener('mousemove', hideCursorLater));

    volume.addEventListener('mousedown',  onVolumeSelect);
    volume.addEventListener('touchstart', onVolumeSelect);
    volume.addEventListener('touchmove',  onVolumeSelect);
    volume.addEventListener('mousedown', (e) =>
        volume.addEventListener('mousemove', onVolumeSelect));
    volume.addEventListener('mouseup', () =>
        volume.removeEventListener('mousemove', onVolumeSelect));
    volume.addEventListener('mouseleave', () =>
        volume.removeEventListener('mousemove', onVolumeSelect));
    onVolumeChange(view.volume, view.muted);

    root.querySelector('.mute').addEventListener('click', () => {
        view.muted = !view.muted;
    });

    root.querySelector('.theatre').addEventListener('click', () =>
        document.body.classList.add('theatre'));

    root.querySelector('.fullscreen').addEventListener('click', () =>
        screenfull.request(root));

    root.querySelector('.collapse').addEventListener('click', () => {
        document.body.classList.remove('theatre');
        screenfull.exit();
    });

    onLoad();
    return {
        onLoad: (socket) => {
            rpc = socket;
            rpc.connect('Stream.ViewerCount', (n) => {
                info.querySelector('.viewers').textContent = n;
            });
            // TODO measure connection speed, request a stream
            view.src = `/stream/${stream}`;
            view.play();
        },

        onUnload: () => {
            rpc = null;
            view.src = '';
        },
    };
};


let Chat = function (root) {
    let form = root.querySelector('.input-form');
    let text = form.querySelector('.input');
    let log  = root.querySelector('.log');
    let msg  = root.querySelector('.message-template');
    let rpc  = null;

    text.addEventListener('keydown', (ev) =>
        (ev.keyCode === 13 && !ev.shiftKey ? ev.preventDefault() : null));

    text.addEventListener('keyup', (ev) =>
        (ev.keyCode === 13 && !ev.shiftKey ?
            form.dispatchEvent(new Event('submit', {cancelable: true})) : null));

    form.addEventListener('submit', (ev) => {
        ev.preventDefault();
        if (rpc && text.value) {
            rpc.send('Chat.SendMessage', text.value).then(() => {
                log.scrollTop = log.scrollHeight;
                text.value = '';
                text.focus();
            });
        }
    });

    let lform = root.querySelector('.login-form');
    let login = lform.querySelector('.input');

    lform.addEventListener('submit', (ev) => {
        ev.preventDefault();
        if (rpc && login.value) {
            rpc.send('Chat.SetName', login.value);
        }
    });

    return {
        onLoad: (socket) => {
            rpc = socket;
            rpc.connect('Chat.Message', (name, text) => {
                let rect = log.getBoundingClientRect();
                let scroll = log.scrollTop + (rect.bottom - rect.top) >= log.scrollHeight;
                let entry = document.importNode(msg.content, true);
                entry.querySelector('.name').textContent = name;
                entry.querySelector('.text').textContent = text;
                log.appendChild(entry);
                if (scroll)
                    log.scrollTop = log.scrollHeight;
            });

            rpc.connect('Chat.AcquiredName', (name) => {
                root.classList.add('logged-in');
                text.focus();
                log.scrollTop = log.scrollHeight;
            });

            rpc.send('Chat.RequestHistory');
            root.classList.add('online');
        },

        onUnload: () => {
            rpc = null;
            root.classList.remove('online');
        },
    };
};


let stream = document.body.getAttribute('data-stream-id');
let view   = new View(document.querySelector('.player'), document.querySelector('.meta'), stream);
let chat   = new Chat(document.querySelector('.chat'));
let rpc    = new RPC(`ws${window.location.protocol == 'https:' ? 's' : ''}://`
                     + `${window.location.host}/stream/${encodeURIComponent(stream)}`,
                     chat, view);
