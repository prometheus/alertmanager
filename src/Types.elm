module Types exposing (Model, Msg(..), Route(..))

import Alerts.Types exposing (AlertGroup, Alert)
import Views.AlertList.Types exposing (AlertListMsg)
import Views.SilenceList.Types exposing (SilenceListMsg)
import Views.Silence.Types exposing (SilenceMsg)
import Views.SilenceForm.Types exposing (SilenceFormMsg)
import Views.Status.Types exposing (StatusModel, StatusMsg)
import Silences.Types exposing (Silence)
import Utils.Types exposing (ApiData, Filter)
import Time


type alias Model =
    { silences : ApiData (List Silence)
    , silence : ApiData Silence
    , alertGroups : ApiData (List AlertGroup)
    , route : Route
    , filter : Filter
    , currentTime : Time.Time
    , status : StatusModel
    }


type Msg
    = CreateSilenceFromAlert Alert
    | AlertGroupsPreview (ApiData (List AlertGroup))
    | MsgForAlertList AlertListMsg
    | MsgForSilence SilenceMsg
    | MsgForSilenceForm SilenceFormMsg
    | MsgForSilenceList SilenceListMsg
    | MsgForStatus StatusMsg
    | NavigateToAlerts Views.AlertList.Types.Route
    | NavigateToNotFound
    | NavigateToSilence String
    | NavigateToSilenceFormEdit String
    | NavigateToSilenceFormNew
    | NavigateToSilenceList (Maybe String)
    | NavigateToStatus
    | NewUrl String
    | Noop
    | PreviewSilence Silence
    | RedirectAlerts
    | UpdateCurrentTime Time.Time
    | UpdateFilter Filter String


type Route
    = AlertsRoute Views.AlertList.Types.Route
    | NotFoundRoute
    | SilenceFormEditRoute String
    | SilenceFormNewRoute
    | SilenceListRoute (Maybe String)
    | SilenceRoute String
    | StatusRoute
    | TopLevelRoute
