'use strict';


(() => {
    let stream = document.body.getAttribute('data-stream-id');

    let view = document.querySelector('.w-view');
    let wrap = document.querySelector('.w-view-wrap');
    let chat = document.querySelector('.w-chat-container');

    view.addEventListener('loadedmetadata', () => {
        wrap.classList.remove('w-icon-loading');
        wrap.querySelector('.w-view-pad').remove();
    });

    let chat_input = chat.querySelector('.input');
    let chat_form  = chat.querySelector('.input-form');
    let chat_log   = chat.querySelector('.log');
    let chat_msg   = chat.querySelector('.message');

    chat_msg.remove();

    let chat_entry = (name, text, add_class) => {
        let elem = chat_msg.cloneNode(true);
        elem.querySelector('.name').textContent = name;
        elem.querySelector('.text').textContent = text;
        if (add_class)
            elem.classList.add(add_class);
        chat_log.appendChild(elem);
    };

    chat_input.setAttribute('disabled', '');

    chat_input.addEventListener('keydown', (ev) => {
        if (ev.keyCode === 13 && !ev.shiftKey) {
            ev.preventDefault();
        }
    });

    chat_input.addEventListener('keyup', (ev) => {
        if (ev.keyCode === 13 && !ev.shiftKey) {
            chat_form.dispatchEvent(new Event('submit'));
        }
    });

    chat_form.addEventListener('submit', (ev) => {
        ev.preventDefault();

        if (ws.readyState == 1 && chat_input.value) {
            ws.send(chat_input.value);
            chat_input.value = '';
            chat_input.focus();
        }
    });

    let ws = new WebSocket(`ws${window.location.protocol == 'https:' ? 's' : ''}://`
                           + `${window.location.host}/stream/${stream}`);

    ws.onopen = () => {
        // TODO measure connection speed, request a stream
        view.src = `/stream/${stream}`;
        view.play();
        chat_input.removeAttribute('disabled');
    };

    ws.onerror = (ev) => {
        // TODO log something
        wrap.classList.remove('w-icon-loading');
        wrap.classList.add('uk-icon-warning');
        chat_input.setAttribute('disabled', '');
    };

    ws.onclose = (ev) => {
        // TODO
        wrap.classList.remove('w-icon-loading');
        wrap.classList.add('uk-icon-warning');
        chat_input.setAttribute('disabled', '');
    };

    ws.onmessage = (ev) => {
        // TODO
        chat_entry('yoba', ev.data);
    };
})();
