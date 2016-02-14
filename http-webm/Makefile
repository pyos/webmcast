AR       ?= ar
PYTHON   ?= python
CXX      ?= g++
CXXFLAGS ?= -O3


.PHONY: python clean


python: obj/libbroadcast.a
	$(PYTHON) mkffi.py


obj/libbroadcast.a: obj/broadcast.o
	$(AR) rcs $@ $<


obj/%.o: src/%.cc src/%.h
	@mkdir -p obj
	$(CXX) $(CXXFLAGS) -std=c++11 -Wall -Wextra -Werror -fPIC $< -c -o $@


clean:
	rm -rf obj build
