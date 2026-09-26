\ Structural Conditionals, loops part                  12dec92py

\ Authors: Anton Ertl, Bernd Paysan
\ Copyright (C) 1995,1996,1997,1999,2001,2003,2006,2007,2012,2019 Free Software Foundation, Inc.

\ This file is part of Gforth.

\ Gforth is free software; you can redistribute it and/or
\ modify it under the terms of the GNU General Public License
\ as published by the Free Software Foundation, either version 3
\ of the License, or (at your option) any later version.

\ This program is distributed in the hope that it will be useful,
\ but WITHOUT ANY WARRANTY; without even the implied warranty of
\ MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
\ GNU General Public License for more details.

\ You should have received a copy of the GNU General Public License
\ along with this program. If not, see http://www.gnu.org/licenses/.

\ Structural Conditionals                              12dec92py

\ this works with flash

: (leave)   -1 , ;
: LEAVE     postpone branch  (leave) ;  immediate restrict
: ?LEAVE    postpone 0= postpone ?branch  (leave) ;
                                             immediate restrict

: DONE   ( addr -- )  here  swap ?DO
	I @ ['] branch =  I @ ['] ?branch = or  I @ ['] (?do) = or
	I cell+ @ -1 = and  IF  I cell+ >resolve  THEN
    cell +LOOP ;                             immediate restrict

\ Structural Conditionals                              12dec92py

: DO        postpone (do)   here ;            immediate restrict

: ?DO       postpone (?do)  (leave) here ;
                                             immediate restrict
: FOR       postpone (for)  here ;            immediate restrict

: loop]     dup <resolve 2 cells - postpone done postpone unloop ;

: LOOP      sys? postpone (loop)  loop] ;     immediate restrict
: +LOOP     sys? postpone (+loop) loop] ;     immediate restrict
: NEXT      sys? postpone (next)  loop] ;     immediate restrict

: EXIT postpone ;s ; immediate restrict
: ?EXIT postpone IF postpone EXIT postpone THEN ; immediate restrict

