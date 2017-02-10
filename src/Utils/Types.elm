module Utils.Types exposing (..)

import Http
import ISO8601


type ApiResponse e a
    = Loading
    | Failure e
    | Success a


type alias ApiData a =
    ApiResponse Http.Error a


type alias Time =
    { t : ISO8601.Time
    , s : String
    , valid : Bool
    }
