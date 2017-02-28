module Alerts.Types exposing (..)

import Http exposing (Error)
import ISO8601
import Utils.Types exposing (ApiData, Filter)


type Route
    = Receiver (Maybe String) (Maybe Bool) (Maybe String)


type Msg
    = ForSelf AlertsMsg
    | ForParent OutMsg


type OutMsg
    = SilenceFromAlert Alert
    | UpdateFilter Filter String
    | NewUrl String


type AlertsMsg
    = AlertGroupsFetch (ApiData (List AlertGroup))
    | FetchAlertGroups
    | Noop
    | FilterAlerts


type alias Block =
    { alerts : List Alert
    , routeOpts : RouteOpts
    }


type alias RouteOpts =
    { receiver : String }


type alias AlertGroup =
    { blocks : List Block
    , labels : Utils.Types.Labels
    }


type alias Alert =
    { annotations : Utils.Types.Labels
    , labels : Utils.Types.Labels
    , inhibited : Bool
    , silenceId : Maybe String
    , silenced : Bool
    , startsAt : ISO8601.Time
    , generatorUrl : String
    }
