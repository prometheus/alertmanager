module Alerts.Types exposing (Alert, Receiver)

import Time exposing (Time)
import Utils.Types exposing (Labels)


type alias Alert =
    { annotations : Labels
    , labels : Labels
    , silenceId : Maybe String
    , isInhibited : Bool
    , startsAt : Time
    , generatorUrl : String
    , id : String
    }


type alias Receiver =
    { name : String
    , regex : String
    }
