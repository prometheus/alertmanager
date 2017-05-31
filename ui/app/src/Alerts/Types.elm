module Alerts.Types exposing (Alert, AlertGroup, Block, RouteOpts)

import Utils.Types exposing (Labels)
import Time exposing (Time)


-- TODO: Revive inhibited field


type alias Alert =
    { annotations : Labels
    , labels : Labels
    , silenceId : Maybe String
    , isInhibited : Bool
    , startsAt : Time
    , generatorUrl : String
    , id : String
    }


type alias AlertGroup =
    { blocks : List Block
    , labels : Labels
    }


type alias Block =
    { alerts : List Alert
    , routeOpts : RouteOpts
    }


type alias RouteOpts =
    { receiver : String }
