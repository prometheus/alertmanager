module Utils.Types exposing (..)

import Http


type ApiResponse e a
    = NotAsked
    | Loading
    | Failure e
    | Success a


type alias ApiData a =
    ApiResponse Http.Error a
