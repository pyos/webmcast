'use strict'; /* global screenfull, $init, $form */

if (screenfull.enabled)
    document.addEventListener(screenfull.raw.fullscreenchange, _ => {
        if (screenfull.isFullscreen)
            // browser support for :fullscreen is abysmal.
            screenfull.element.classList.add('is-fullscreen');
        else for (let e of document.querySelectorAll('.is-fullscreen'))
            e.classList.remove('is-fullscreen');
    });
else
    document.body.classList.add('no-fullscreen');


Element.prototype.insertThisBefore = function (e) {
    return e.parentElement.insertBefore(this, e);
};


Element.prototype.button = function (selector, f) {
    for (let e of this.querySelectorAll(selector)) {
        e.addEventListener('click', ev => ev.preventDefault());
        e.addEventListener('click', f);
    }
};


let RPC = function(url) {
    this.nextID   = 0;
    this.events   = {};
    this.requests = {};
    this.objects  = [];
    this.socket   = new WebSocket(url);
    this.state    = 0;

    this.socket.onopen = _ => {
        for (let object of this.objects)
            object.load();
        this.state = 1;
    };

    this.socket.onclose = _ => {
        for (let object of this.objects)
            object.unload();
        this.state = 2;
    };

    this.socket.onmessage = ev => {
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


RPC.prototype.register = function (obj) {
    if (this.state === 1)
        obj.load();
    if (this.state === 2)
        obj.unload();
    this.objects.push(obj);
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


let delayedPair = (delay, f, g) => {
    let t;
    return _ => {
        if (t === undefined)
            f();
        else
            window.clearTimeout(t);
        t = window.setTimeout(() => { t = undefined; g(); }, delay);
    };
};


let getParentStream = e => {
    for (; e !== null; e = e.parentElement)
        if (e.rpc !== undefined)
            return e;
    return null;
};


$init = Object.assign($init, {
    '[data-stream-id]'(e) {
        let server = e.dataset.server || location.host;
        if (server[0] === ':')
            server = location.hostname + server;
        e.url = `${location.protocol}//${server}/stream/${encodeURIComponent(e.dataset.streamId)}`;
        e.rpc = new RPC(e.url.replace('http', 'ws'));
    },

    '.player-block'(e) {
        e.button('.theatre',  _ => e.classList.add('theatre'));
        e.button('.collapse', _ => e.classList.remove('theatre'));
    },

    '.player'(e) {
        // TODO playing, waiting, stalled (not sure whether these events are actually emitted)
        let video  = e.querySelector('video');
        let status = e.querySelector('.status');
        let volume = e.querySelector('.volume');

        let setStatus = (short, long) => {
            e.dataset.status = short;
            status.textContent = long || short;
        };

        let onTimeUpdate = t =>
            // let leftPad = require('left-pad');
            setStatus('playing', `${(t / 60)|0}:${t % 60 < 10 ? '0' : ''}${(t|0) % 60}`);

        let onError = code => setStatus('error',
              code === 1 ? 'aborted'
            : code === 2 ? 'network error'
            : code === 3 ? 'decoding error'
            : code === 4 ? 'stream ended'
            : 'unknown error');

        video.addEventListener('loadstart',      _ => setStatus('loading'));
        video.addEventListener('loadedmetadata', _ => setStatus('loading', 'buffering'));
        video.addEventListener('timeupdate',     _ => onTimeUpdate(video.currentTime));
        video.addEventListener('ended',          _ => onError(4 /* "unsupported media" */));
        video.addEventListener('error',          _ => onError(video.error.code));

        let showControls = delayedPair(3000,
            () => e.classList.remove('hide-controls'),
            () => e.classList.add('hide-controls'));

        e.addEventListener('mousemove', showControls);
        e.addEventListener('focusin',   showControls);
        e.addEventListener('keydown',   showControls);
        e.button('.mute',       _ => video.muted = !video.muted);
        e.button('.fullscreen', _ => screenfull.request(e));
        e.button('.collapse',   _ => screenfull.exit());

        let onVolumeChange = _ => {
            let s = volume.querySelector('.slider');
            s.style.right = `${100 - video.volume * 100}%`;
            s.style.top   = `${100 - video.volume * 100}%`;
            if (video.muted)
                e.classList.add('muted');
            else
                e.classList.remove('muted');
        };

        let onVolumeSelect = ev => {
            ev.preventDefault();
            let r = volume.getBoundingClientRect();
            let x = ((ev.touches ? ev.touches[0].clientX : ev.clientX) - r.left) / (r.right - r.left);
            video.volume = Math.min(1, Math.max(0, x));
            video.muted  = false;
        };

        video.addEventListener('volumechange', onVolumeChange);
        // when styling <input type="range"> is too hard
        volume.addEventListener('mousedown',  onVolumeSelect);
        volume.addEventListener('touchstart', onVolumeSelect);
        volume.addEventListener('touchmove',  onVolumeSelect);
        volume.addEventListener('mousedown',  _ => volume.addEventListener('mousemove', onVolumeSelect));
        volume.addEventListener('mouseup',    _ => volume.removeEventListener('mousemove', onVolumeSelect));
        volume.addEventListener('mouseleave', _ => volume.removeEventListener('mousemove', onVolumeSelect));
        volume.addEventListener('keydown',    ev =>
            video.volume = ev.keyCode === 37 ? Math.max(0, video.volume - 0.05)  // left arrow
                         : ev.keyCode === 39 ? Math.min(1, video.volume + 0.05)  // right arrow
                         : video.volume);
        onVolumeChange(null);

        let stream = getParentStream(e);
        if (stream) {
            setStatus('loading');
            stream.rpc.register({
                load() {
                    // TODO measure connection speed, request a stream
                    video.src = stream.url;
                    video.play();
                },

                unload() {
                    video.src = '';
                },
            });
        }
    },

    '.stream-info .user-header'(e) {
        e.button('.edit', ev => {
            let name = e.querySelector('.name');
            let t = $init.template('edit-name-template');
            let f = t.querySelector('form');
            let i = f.querySelector('input');
            f.addEventListener('reset',  _  => f.remove());
            f.addEventListener('submit', ev => {
                ev.preventDefault();
                if (i.value === '')
                    return f.remove();
                $form.submit(f).then(() => {
                    name.textContent = i.value;
                    f.remove();
                });
            });
            f.insertThisBefore(ev.currentTarget);
            i.value = name.textContent;
            i.focus();
        });

        let stream = getParentStream(e);
        if (stream)
            stream.rpc.connect('Stream.ViewerCount', n =>
                e.querySelector('.viewers').textContent = n);
    },

    '.stream-info .about'(e) {
        e.button('.edit', ev => {
            let t = $init.template('edit-panel-template');
            let f = t.querySelector('form');
            let i = f.querySelector('textarea');
            f.addEventListener('reset', _ => f.remove());

            let id = ev.currentTarget.dataset.panel;
            if (id) {
                f.querySelector('[name="id"]').value = id;
                f.querySelector('.remove').addEventListener('click', () => {
                    f.setAttribute('action', '/user/del-stream-panel');
                    f.dispatchEvent(new Event('submit'));
                });
            } else {
                f.querySelector('.remove').remove();
            }

            f.insertThisBefore(ev.currentTarget);
            i.value = ev.currentTarget.parentElement.querySelector('[data-markup]').textContent;
            i.focus();
        });
    },

    '.chat'(root) {
        let rpc  = getParentStream(root).rpc;
        let log  = root.querySelector('.log');
        let err  = root.querySelector('.error-message');
        let form = root.querySelector('.input-form');
        let text = root.querySelector('.input-form .input');

        let autoscroll = (domModifier) => {
            let atBottom = log.scrollTop + log.clientHeight >= log.scrollHeight;
            domModifier();
            if (atBottom)
                log.scrollTop = log.scrollHeight;
        };

        let handleErrors = (form, promise, withMessage) => {
            $form.disable(form);
            return promise.then(() => {
                $form.enable(form);
                form.classList.remove('error');
                err.classList.remove('visible');
                err.textContent = '';
            }).catch((e) => {
                $form.enable(form);
                form.classList.add('error');
                if (withMessage) {
                    form.appendChild(err);
                    err.textContent = e.message;
                    err.classList.add('visible');
                }
                throw e;
            });
        };

        err.addEventListener('click', () =>
            err.classList.remove('visible'));

        root.querySelector('.login-form').addEventListener('submit', function (ev) {
            ev.preventDefault();
            // TODO catch errors
            handleErrors(this, rpc.send('Chat.SetName', this.querySelector('.input').value));
        });

        form.addEventListener('submit', (ev) => {
            ev.preventDefault();
            // TODO catch errors
            handleErrors(form, rpc.send('Chat.SendMessage', text.value), true).then(() => {
                log.scrollTop = log.scrollHeight;
                text.value = '';
                text.select();
            });
        });

        text.addEventListener('keydown', (ev) => {
            if (ev.keyCode === 13 && !ev.shiftKey) {  // carriage return
                ev.preventDefault();
                form.dispatchEvent(new Event('submit', {cancelable: true}));
            }
        });

        let stringColor = (str) => {
            let h = 0;
            for (let i = 0; i < str.length; i++)
                // Java's `String.hashCode`.
                h = h * 31 + str.charCodeAt(i) | 0;
            let s = [30, 50, 70, 90];
            let l = [60, 70, 80, 90];
            return `hsl(${h % 359},${s[(h / 359|0) % s.length]}%,${l[((h / 359|0) / s.length|0) % l.length]}%)`;
        };

        rpc.connect('Chat.Message', (name, text, login, isReal) => {
            autoscroll(() => {
                let entry = $init.template('chat-message-template');
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
                if (login === "") {
                    root.classList.remove('logged-in');
                    root.querySelector('.login-form').classList.add('error');
                } else {
                    root.classList.add('logged-in');
                    text.select();
                }
            });
        });

        rpc.register({
            load() {
                rpc.send('Chat.RequestHistory');
                root.classList.add('online');
            },

            unload() {
                root.classList.remove('online');
            },
        });
    },
});
