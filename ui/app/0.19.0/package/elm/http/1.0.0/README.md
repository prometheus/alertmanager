# HTTP in Elm

Make HTTP requests in Elm.

```elm
import Http
import Json.Decode as Decode


-- GET A STRING

getWarAndPeace : Http.Request String
getWarAndPeace =
  Http.getString "https://example.com/books/war-and-peace"


-- GET JSON

getMetadata : Http.Request Metadata
getMetadata =
  Http.get "https://example.com/books/war-and-peace/metadata" decodeMetadata

type alias Metadata =
  { author : String
  , pages : Int
  }

decodeMetadata : Decode.Decoder Metadata
decodeMetadata =
  Decode.map2 Metadata
    (Decode.field "author" Decode.string)
    (Decode.field "pages" Decode.int)


-- SEND REQUESTS

type Msg
  = LoadMetadata (Result Http.Error Metadata)

send : Cmd Msg
send =
  Http.send LoadMetadata getMetadata
```


## Examples

  - GET requests - [demo and code](http://elm-lang.org/examples/http)
  - Download progress - [demo](https://hirafuji.com.br/elm/http-progress-example/) and [code](https://gist.github.com/pablohirafuji/fa373d07c42016756d5bca28962008c4)


## Learn More

To understand how HTTP works in Elm, check out:

  - [The HTTP example in the guide](https://guide.elm-lang.org/architecture/effects/http.html) to see a simple usage with some explanation.
  - [The Elm Architecture](https://guide.elm-lang.org/architecture/) to understand how HTTP fits into Elm in a more complete way. This will explain concepts like `Cmd` and `Sub` that appear in this package.
