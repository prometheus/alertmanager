# JSON in Elm

This package helps you convert between Elm values and JSON values.

This package is usually used alongside [`elm/http`](http://package.elm-lang.org/packages/elm/http/latest) to talk to servers or [ports](https://guide.elm-lang.org/interop/ports.html) to talk to JavaScript.


## Example

Have you seen this [causes of death](https://en.wikipedia.org/wiki/List_of_causes_of_death_by_rate) table? Did you know that in 2002, war accounted for 0.3% of global deaths whereas road traffic accidents accounted for 2.09% and diarrhea accounted for 3.15%?

The table is interesting, but say we want to visualize this data in a nicer way. We will need some way to get the cause-of-death data from our server, so we create encoders and decoders:

```elm
module Cause exposing (Cause, encode, decoder)

import Json.Decode as D
import Json.Encode as E


-- CAUSE OF DEATH

type alias Cause =
  { name : String
  , percent : Float
  , per100k : Float
  }


-- ENCODE

encode : Cause -> E.Value
encode cause =
  E.object
    [ ("name", E.string cause.name)
    , ("percent", E.float cause.percent)
    , ("per100k", E.float cause.per100k)
    ]


-- DECODER

decoder : D.Decoder Cause
decoder =
  D.map3 Cause
    (D.field "name" D.string)
    (D.field "percent" D.float)
    (D.field "per100k" D.float)
```

Now in some other code we can use `Cause.encode` and `Cause.decoder` as building blocks. So if we want to decode a list of causes, saying `Decode.list Cause.decoder` will handle it!

Point is, the goal should be:

  1. Make small JSON decoders and encoders.
  2. Snap together these building blocks as needed.

So say you decide to make the `name` field more precise. Instead of a `String`, you want to use codes from the [International Classification of Diseases](http://www.who.int/classifications/icd/en/) recommended by the World Health Organization. These [codes](http://apps.who.int/classifications/icd10/browse/2016/en) are used in a lot of mortality data sets. So it may make sense to make a separate `IcdCode` module with its own `IcdCode.encode` and `IcdCode.decoder` that ensure you are working with valid codes. From there, you can use them as building blocks in the `Cause` module!


## Future Plans

It is easy to get focused on how to optimize the use of JSON, but I think this is missing the bigger picture. Instead, I would like to head towards [this vision](https://gist.github.com/evancz/1c5f2cf34939336ecb79b97bb89d9da6) of data interchange.
