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
    | NavigateToAlerts Filter
    | NavigateToNotFound
    | NavigateToSilence String
    | NavigateToSilenceFormEdit String
    | NavigateToSilenceFormNew
    | NavigateToSilenceList Filter
    | NavigateToStatus
    | NewUrl String
    | Noop
    | PreviewSilence Silence
    | RedirectAlerts
    | UpdateCurrentTime Time.Time
    | UpdateFilter Filter String


type Route
    = AlertsRoute Filter
    | NotFoundRoute
    | SilenceFormEditRoute String
    | SilenceFormNewRoute
    | SilenceListRoute Filter
    | SilenceRoute String
    | StatusRoute
    | TopLevelRoute
