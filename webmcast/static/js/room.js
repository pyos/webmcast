'use strict';


(() => {
    let stream = document.body.getAttribute('data-stream-id');

    let view = document.querySelector('.w-view-container .view');
    let chat = document.querySelector('.w-chat-container');

    let chat_input = chat.querySelector('.input');
    let chat_form  = chat.querySelector('.input-form');
    let chat_log   = chat.querySelector('.log');
    let chat_msg   = chat.querySelector('.message');

    chat_input.setAttribute('disabled', '');
    chat_msg.remove();

    let chat_entry = (name, text, add_class) => {
        let elem = chat_msg.cloneNode(true);
        elem.querySelector('.name').textContent = name;
        elem.querySelector('.text').textContent = text;
        if (add_class)
            elem.classList.add(add_class);
        chat_log.appendChild(elem);
    };

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
        // TODO destroy the view
        chat_entry('', 'connection error', 'status');
    };

    ws.onclose = (ev) => {
        // TODO
        chat_input.setAttribute('disabled', '');
        chat_entry('', 'disconnected', 'status');
    };

    ws.onmessage = (ev) => {
        // TODO
        chat_entry('random', ev.data);
    };
})();
