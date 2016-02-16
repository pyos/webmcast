PYTHON ?= python


.PHONY: clean


webm_stream/c.o: mkffi.py src/broadcast.c src/broadcast.h src/buffer.h src/binary.h src/rewriting.h
	$(PYTHON) mkffi.py


clean:
	rm -rf obj/c.*
