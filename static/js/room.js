'use strict'; /* global screenfull, $init, $form */

if (screenfull.enabled) {
    document.addEventListener(screenfull.raw.fullscreenchange, () => {
        if (screenfull.isFullscreen)
            // browser support for :fullscreen is abysmal.
            screenfull.element.classList.add('is-fullscreen');
        else
            document.querySelector('.is-fullscreen').classList.remove('is-fullscreen');
    });

    document.addEventListener(screenfull.raw.fullscreenerror, () =>
        document.body.classList.add('no-fullscreen'));
} else {
    document.body.classList.add('no-fullscreen');
}


let RPC = function(url) {
    this.nextID   = 0;
    this.events   = {};
    this.requests = {};
    this.objects  = [];
    this.socket   = new WebSocket(url);

    this.socket.onopen = () => {
        for (let object of this.objects)
            object.onLoad();
    };

    this.socket.onclose = (ev) => {
        for (let object of this.objects)
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
    let id = this.nextID++ & 0xFFFF;
    this.socket.send(JSON.stringify({ jsonrpc: '2.0', id, method, params }));
    return new Promise((resolve, reject) => { this.requests[id] = { resolve, reject }; });
};


RPC.prototype.connect = function (event, cb) {
    if (cb === undefined)
        delete this.events[event];
    else
        this.events[event] = cb;
};


let View = function (rpc, root, uri) {
    let video  = root.querySelector('video');
    let status = root.querySelector('.status');
    let volume = root.querySelector('.volume');

    let onVolumeSelect = (e) => {
        e.preventDefault();
        let r = volume.getBoundingClientRect();
        let x = Math.min(r.right, Math.max(r.left, e.touches ? e.touches[0].clientX : e.clientX));
        video.volume = (x - r.left) / (r.right - r.left);
        video.muted  = false;
    };

    let onVolumeChange = () => {
        let e = volume.querySelector('.slider');
        let r = volume.getBoundingClientRect();
        e.style.left = `${video.volume * (r.right - r.left)}px`;
        e.style.top = `${(1 - video.volume) * (r.bottom - r.top)}px`;
        if (video.muted)
            root.classList.add('muted');
        else
            root.classList.remove('muted');
    };

    let onTimeUpdate = () => {
        // let leftPad = require('left-pad');
        let t = video.currentTime;
        status.textContent = `${(t / 60)|0}:${t % 60 < 10 ? '0' : ''}${(t|0) % 60}`;
    };

    let onDone = () => {
        root.setAttribute('data-status',
            video.error === null || video.error.code === 4 ? 'ended' : 'error');
        status.textContent = video.error === null   ? 'stream ended'
                           : video.error.code === 1 ? 'aborted'
                           : video.error.code === 2 ? 'network error'
                           : video.error.code === 3 ? 'decoding error'
                           : /* video.error.code === 4 ? */ 'stream ended';
    };

    let onLoadStart = () => {
        root.setAttribute('data-status', 'loading');
        status.textContent = 'loading';
    };

    let onLoadEnd = () => {
        root.setAttribute('data-status', 'playing');
        status.textContent = 'playing';
    };

    let hideCursorTimeout = null;
    let hideCursorLater = () => {
        showCursor();
        hideCursorTimeout = window.setTimeout(() =>
            root.classList.add('hide-cursor'), 3000);
    };

    let showCursor = () => {
        window.clearTimeout(hideCursorTimeout);
        root.classList.remove('hide-cursor');
    };

    video.addEventListener('mouseenter',     hideCursorLater);
    video.addEventListener('mousemove',      hideCursorLater);
    video.addEventListener('mouseleave',     showCursor);
    // TODO playing, waiting, stalled (not sure whether these events are actually emitted)
    video.addEventListener('loadstart',      onLoadStart);
    video.addEventListener('loadedmetadata', onLoadEnd);
    video.addEventListener('error',          onDone);
    video.addEventListener('ended',          onDone);
    video.addEventListener('timeupdate',     onTimeUpdate);
    video.addEventListener('volumechange',   onVolumeChange);
    // when styling <input type="range"> is too hard
    volume.addEventListener('mousedown',  onVolumeSelect);
    volume.addEventListener('touchstart', onVolumeSelect);
    volume.addEventListener('touchmove',  onVolumeSelect);
    volume.addEventListener('mousedown',  () => volume.addEventListener('mousemove', onVolumeSelect));
    volume.addEventListener('mouseup',    () => volume.removeEventListener('mousemove', onVolumeSelect));
    volume.addEventListener('mouseleave', () => volume.removeEventListener('mousemove', onVolumeSelect));
    volume.addEventListener('keydown', (ev) => {
        if (ev.keyCode === 37)  // left arrow
            video.volume = Math.max(0, video.volume - 0.05);
        if (ev.keyCode == 39)  // right arrow
            video.volume = Math.min(1, video.volume + 0.05);
    });

    root.querySelector('.mute').addEventListener('click', (ev) => {
        ev.preventDefault();
        video.muted = !video.muted;
    });

    root.querySelector('.theatre').addEventListener('click', (ev) => {
        ev.preventDefault();
        document.body.classList.add('theatre');
    });

    root.querySelector('.fullscreen').addEventListener('click', (ev) => {
        ev.preventDefault();
        screenfull.request(root);
    });

    root.querySelector('.collapse').addEventListener('click', (ev) => {
        ev.preventDefault();
        document.body.classList.remove('theatre');
        screenfull.exit();
    });

    onVolumeChange();
    onLoadStart();
    return {
        onLoad: () => {
            video.src = uri;  // TODO measure connection speed, request a stream
            video.play();
        },

        onUnload: () => {
            video.src = '';
        },
    };
};


let Chat = function (rpc, root) {
    let log  = root.querySelector('.log');
    let msg  = root.querySelector('.message-template');
    let form = root.querySelector('.input-form');
    let text = root.querySelector('.input-form .input');

    let autoscroll = (domModifier) => {
        let atBottom = log.scrollTop + log.clientHeight >= log.scrollHeight;
        domModifier();
        if (atBottom)
            log.scrollTop = log.scrollHeight;
    };

    root.querySelector('.login-form').addEventListener('submit', function (ev) {
        ev.preventDefault();
        // TODO catch errors
        rpc.send('Chat.SetName', this.querySelector('.input').value);
    });

    form.addEventListener('submit', (ev) => {
        ev.preventDefault();
        // TODO catch errors
        rpc.send('Chat.SendMessage', text.value).then(() => {
            log.scrollTop = log.scrollHeight;
            text.value = '';
            text.select();
        });
    });

    text.addEventListener('keydown', (ev) =>
        // do not input line breaks without shift
        ev.keyCode === 13 && !ev.shiftKey ? ev.preventDefault() : null);

    text.addEventListener('keyup', (ev) =>
        // send the message on Enter (but not Shift+Enter)
        ev.keyCode === 13 && !ev.shiftKey ?
            form.dispatchEvent(new Event('submit', {cancelable: true})) : null);

    let stringColor = (str) => {
        let h = 0;
        for (let i = 0; i < str.length; i++)
            // Java's `String.hashCode`.
            // h = (uint32_t) (h * 31 + s[i])
            h = ((h << 5) - h + str.charCodeAt(i))|0;
        let s = [30, 50, 70, 90];
        let l = [60, 70, 80, 90];
        return `hsl(${h % 359},${s[(h / 359|0) % s.length]}%,${l[((h / 359|0) / s.length|0) % l.length]}%)`;
    };

    rpc.connect('Chat.Message', (name, text, login, isReal) => {
        autoscroll(() => {
            console.log(stringColor(login));
            let entry = document.importNode(msg.content, true);
            let e = entry.querySelector('.name');
            // TODO maybe do this server-side? that'd allow us to hash the IP instead...
            e.style.color = stringColor(login);
            e.textContent = name;
            if (!isReal) {
                e.setAttribute('title', 'Anonymous user');
                e.classList.add('anon');
            } else {
                e.setAttribute('title', login);
            }
            entry.querySelector('.text').textContent = text;
            log.appendChild(entry);
        });
    });

    rpc.connect('Chat.AcquiredName', (name, login) => {
        autoscroll(() => {
            if (name === "") {
                root.classList.remove('logged-in');
            } else {
                root.classList.add('logged-in');
                text.select();
            }
        });
    });

    return {
        onLoad: () => {
            rpc.send('Chat.RequestHistory');
            root.classList.add('online');
        },

        onUnload: () => {
            root.classList.remove('online');
        },
    };
};


let Meta = function (rpc, meta, about, stream) {
    rpc.connect('Stream.ViewerCount', (n) => {
        meta.querySelector('.viewers').textContent = n;
    });

    let createNameEditor = (ev) => {
        ev.preventDefault();

        let name = meta.querySelector('.name');
        let t = $init.all(document.importNode(document.getElementById('edit-name-template').content, true));
        let f = t.querySelector('form');
        let i = f.querySelector('input');
        f.addEventListener('reset', () => f.remove());
        f.addEventListener('submit', (ev) => {
            ev.preventDefault();
            $form.submit(f).then(() => {
                name.textContent = i.value || '#' + stream;
                f.remove();
            });
        });
        ev.currentTarget.parentElement.insertBefore(f, ev.currentTarget);
        i.value = name.textContent;
        i.focus();
    };

    let createPanelEditor = (ev) => {
        ev.preventDefault();

        let t = $init.all(document.importNode(document.getElementById('edit-panel-template').content, true));
        let f = t.querySelector('form');
        let i = f.querySelector('textarea');
        f.addEventListener('reset', () => f.remove());

        let id = ev.currentTarget.getAttribute('data-panel');
        if (id === null) {
            f.querySelector('.remove').remove();
        } else {
            f.querySelector('[name="id"]').value = id;
            f.querySelector('.remove').addEventListener('click', () => {
                f.setAttribute('action', '/user/del-stream-panel');
                f.dispatchEvent(new Event('submit'));
            });
        }

        ev.currentTarget.parentElement.insertBefore(f, ev.currentTarget);
        i.value = ev.currentTarget.parentElement.querySelector('[data-markup]').textContent;
        i.focus();
    };

    for (let e of meta.querySelectorAll('.edit'))
        e.addEventListener('click', createNameEditor);
    for (let e of about.querySelectorAll('.edit'))
        e.addEventListener('click', createPanelEditor);
    return { onLoad: () => {}, onUnload: () => {} };
};


$init['[data-stream-id]'] = (root) => {
    let stream = root.getAttribute('data-stream-id');
    let server = root.getAttribute('data-server');
    let owned  = root.hasAttribute('data-owned');

    let uri = ( server    === ''  ? window.location.host
              : server[0] === ':' ? window.location.hostname + server
              : server ) + '/stream/' + encodeURIComponent(stream);
    let rpc = new RPC(`ws${window.location.protocol == 'https:' ? 's' : ''}://` + uri);
    for (let e of root.querySelectorAll('.player'))
        rpc.objects.push(new View(rpc, e, window.location.protocol + '//' + uri));
    for (let e of root.querySelectorAll('.chat'))
        rpc.objects.push(new Chat(rpc, e));
    for (let e of root.querySelectorAll('.stream-info'))
        rpc.objects.push(new Meta(rpc, e.querySelector('.user-header'), e.querySelector('.about'), stream));
};
