module Types exposing (Model, Msg(..), Route(..))

import Browser.Navigation exposing (Key)
import Utils.Filter exposing (Filter, SilenceFormGetParams)
import Utils.Types exposing (ApiData)
import Views.AlertList.Types as AlertList exposing (AlertListMsg)
import Views.Settings.Types as SettingsView exposing (SettingsMsg)
import Views.SilenceForm.Types as SilenceForm exposing (SilenceFormMsg)
import Views.SilenceList.Types as SilenceList exposing (SilenceListMsg)
import Views.SilenceView.Types as SilenceView exposing (SilenceViewMsg)
import Views.Status.Types exposing (StatusModel, StatusMsg)


type alias Model =
    { silenceList : SilenceList.Model
    , silenceView : SilenceView.Model
    , silenceForm : SilenceForm.Model
    , alertList : AlertList.Model
    , route : Route
    , filter : Filter
    , status : StatusModel
    , basePath : String
    , apiUrl : String
    , libUrl : String
    , bootstrapCSS : ApiData String
    , fontAwesomeCSS : ApiData String
    , elmDatepickerCSS : ApiData String
    , defaultCreator : String
    , expandAll : Bool
    , key : Key
    , settings : SettingsView.Model
    }


type Msg
    = MsgForAlertList AlertListMsg
    | MsgForSilenceView SilenceViewMsg
    | MsgForSilenceForm SilenceFormMsg
    | MsgForSilenceList SilenceListMsg
    | MsgForStatus StatusMsg
    | MsgForSettings SettingsMsg
    | NavigateToAlerts Filter
    | NavigateToNotFound
    | NavigateToSilenceView String
    | NavigateToSilenceFormEdit String
    | NavigateToSilenceFormNew SilenceFormGetParams
    | NavigateToSilenceList Filter
    | NavigateToStatus
    | NavigateToSettings
    | NavigateToInternalUrl String
    | NavigateToExternalUrl String
    | RedirectAlerts
    | BootstrapCSSLoaded (ApiData String)
    | FontAwesomeCSSLoaded (ApiData String)
    | ElmDatepickerCSSLoaded (ApiData String)
    | SetDefaultCreator String
    | SetGroupExpandAll Bool


type Route
    = AlertsRoute Filter
    | NotFoundRoute
    | SilenceFormEditRoute String
    | SilenceFormNewRoute SilenceFormGetParams
    | SilenceListRoute Filter
    | SilenceViewRoute String
    | StatusRoute
    | TopLevelRoute
    | SettingsRoute
