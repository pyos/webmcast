AR ?= ar
CXX ?= g++
CXXFLAGS ?= -O3


.PHONY: all clean


all: broadcast_ffi.o


clean:
	rm -rf obj broadcast_ffi.*


broadcast_ffi.o: obj/libbroadcast.a
	python broadcast_ffi_gen.py


obj/%.o: %.cc %.h
	@mkdir -p obj
	$(CXX) $(CXXFLAGS) -std=c++11 -Wall -Wextra -Werror -fPIC $< -c -o $@


obj/libbroadcast.a: obj/broadcast.o
	$(AR) rcs $@ $<
