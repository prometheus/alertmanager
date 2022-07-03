module Types exposing (Model, Msg(..), Route(..), getUser)

import Browser.Navigation exposing (Key)
import Data.User exposing (User)
import Utils.Api
import Utils.Filter exposing (Filter, SilenceFormGetParams)
import Utils.Types exposing (ApiData(..))
import Views.AlertList.Types as AlertList exposing (AlertListMsg)
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
    , username : Maybe String
    , key : Key
    }


type Msg
    = MsgForAlertList AlertListMsg
    | MsgForSilenceView SilenceViewMsg
    | MsgForSilenceForm SilenceFormMsg
    | MsgForSilenceList SilenceListMsg
    | MsgForStatus StatusMsg
    | NavigateToAlerts Filter
    | NavigateToNotFound
    | NavigateToSilenceView String
    | NavigateToSilenceFormEdit String
    | NavigateToSilenceFormNew SilenceFormGetParams
    | NavigateToSilenceList Filter
    | NavigateToStatus
    | NavigateToInternalUrl String
    | NavigateToExternalUrl String
    | RedirectAlerts
    | BootstrapCSSLoaded (ApiData String)
    | FontAwesomeCSSLoaded (ApiData String)
    | ElmDatepickerCSSLoaded (ApiData String)
    | SetDefaultCreator String
    | SetGroupExpandAll Bool
    | UserFetch (ApiData User)


type Route
    = AlertsRoute Filter
    | NotFoundRoute
    | SilenceFormEditRoute String
    | SilenceFormNewRoute SilenceFormGetParams
    | SilenceListRoute Filter
    | SilenceViewRoute String
    | StatusRoute
    | TopLevelRoute


getUser : String -> (ApiData User -> msg) -> Cmd msg
getUser apiUrl msg =
    let
        url =
            String.join "/" [ apiUrl, "me" ]
    in
    Utils.Api.send (Utils.Api.get url Data.User.decoder)
        |> Cmd.map msg
