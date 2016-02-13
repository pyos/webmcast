/* globals base64js */
"use strict";


navigator.getUserMedia = navigator.getUserMedia
                      || navigator.msGetUserMedia
                      || navigator.mozGetUserMedia
                      || navigator.webkitGetUserMedia;


const restream = {
    websocket: (path, host) => {
        if (host === undefined)
            host = location.host;

        return new WebSocket((location.protocol === 'https:' ? 'wss:' : 'ws:') + host + path);
    },

    create: (stream, success, error) => {
        let renderer = document.createElement('video');
        let buffer   = document.createElement('canvas');
        let socket   = restream.websocket('/stream/');
        let handle   = restream.handle.new({ socket, stream });

        renderer.onloadedmetadata = () => {
            let context = buffer.getContext('2d');
            let frames  = 0;
            buffer.width  = renderer.videoWidth;
            buffer.height = renderer.videoHeight;

            const f = () => {
                if (!handle.active) return;
                context.drawImage(renderer, 0, 0);
                socket.send(buffer.toDataURL('image/jpeg', 0.5));
                frames += 1;
            };

            handle.interval(1000, () => {
                console.log('send fps is ', frames);
                frames = 0;
            });

            handle.interval(1000 / 30, () => requestAnimationFrame(f));
            success(handle);
        };

        socket.onmessage = (e) => {
            // TODO other kinds of messages (most importantly, errors.)
            handle.id = e.data;
            renderer.src = URL.createObjectURL(stream);
            renderer.play();
        };

        socket.onclose = () => {
            if (handle !== null)
                handle.close();
            else
                error('could not create a source connection');
        };
    },

    join: (id, display, success, error) => {
        let socket = restream.websocket('/stream/' + id);
        let handle = restream.handle.new({ id, socket });
        let frames = 0;

        socket.onmessage = (e) => {
            // TODO buffer some frames
            requestAnimationFrame(() => display.src = e.data);
            frames += 1;
        };

        handle.interval(1000, () => {
            console.log('recv fps is ', frames);
            frames = 0;
        });

        socket.onclose = () => {
            if (handle !== null)
                handle.close();
            else
                error('could not create a client connection');
        };

        success(handle);
    },

    handle: {
        onclose: null,

        new: function (fields) {
            let c = { active: true, intervals: [] };

            for (let f in fields)
                if (fields.hasOwnProperty(f))
                    c[f] = fields[f];

            Object.setPrototypeOf(c, this);
            return c;
        },

        interval: function(t, f) {
            this.intervals.push(window.setInterval(f, t));
        },

        close: function () {
            if (!this.active)
                return;

            if (this.stream !== undefined)
                for (let track of this.stream.getTracks())
                    track.stop();

            for (let interval of this.intervals)
                clearInterval(interval);

            if (this.socket !== undefined)
                this.socket.close();

            if (this.onclose !== null)
                this.onclose();

            this.active = false;
        },
    },
};


navigator.getUserMedia({video: true},
    (stream) => {
        console.log('acquired', window.stream = stream);

        restream.create(stream,
            (handle) => {
                window.handle1 = handle;
                handle.onclose = () => console.log('h1 closed');
                restream.join(handle.id, document.querySelector('#display'),
                    (handle) => {
                        window.handle2 = handle;
                        handle.onclose = () => console.log('h2 closed');
                    },
                    (error) => console.log(error)
                );
            },
            (error) => console.log(error)
        );
    },
    (error) => {
        console.log('rejected', error);
    }
);
