# Core Libraries

Every Elm project needs this package!

It provides **basic functionality** like addition and subtraction as well as **data structures** like lists, dictionaries, and sets.

> **New to Elm?** Go to [elm-lang.org](http://elm-lang.org) for an overview.


## Default Imports

The modules in this package are so common, that some of them are imported by default in all Elm files. So it is as if every Elm file starts with these imports:

```elm
import Basics exposing (..)
import List exposing (List, (::))
import Maybe exposing (Maybe(..))
import Result exposing (Result(..))
import String exposing (String)
import Char exposing (Char)
import Tuple

import Debug

import Platform exposing ( Program )
import Platform.Cmd as Cmd exposing ( Cmd )
import Platform.Sub as Sub exposing ( Sub )
```

The intention is to include things that are both extremely useful and very unlikely to overlap with anything that anyone will ever write in a library. By keeping the set of default imports relatively small, it also becomes easier to use whatever version of `map` suits your fancy. Finally, it makes it easier to figure out where the heck a function is coming from.
