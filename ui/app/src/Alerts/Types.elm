module Alerts.Types exposing (Alert, Receiver)

import Utils.Types exposing (Labels)
import Time exposing (Time)


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
