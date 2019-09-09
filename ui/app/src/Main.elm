module Main exposing (main)

import Browser exposing (UrlRequest(..))
import Browser.Navigation exposing (Key)
import Json.Decode as Json
import Parsing
import Task
import Time
import Types exposing (Model, Msg(..), Route(..))
import Updates exposing (update)
import Url exposing (Url)
import Utils.Api as Api
import Utils.Filter exposing (nullFilter)
import Utils.Types exposing (ApiData(..))
import Views
import Views.AlertList.Types exposing (initAlertList)
import Views.SilenceForm.Types exposing (initSilenceForm)
import Views.SilenceList.Types exposing (initSilenceList)
import Views.SilenceView.Types exposing (initSilenceView)
import Views.Status.Types exposing (StatusModel, initStatusModel)


main : Program Json.Value Model Msg
main =
    Browser.application
        { init = init
        , update = update
        , view =
            \model ->
                { title = "Alertmanager"
                , body = [ Views.view model ]
                }
        , subscriptions = subscriptions
        , onUrlRequest =
            \request ->
                case request of
                    Internal url ->
                        NavigateToInternalUrl (Url.toString url)

                    External url ->
                        NavigateToExternalUrl url
        , onUrlChange = urlUpdate
        }


init : Json.Value -> Url -> Key -> ( Model, Cmd Msg )
init flags url key =
    let
        route =
            Parsing.urlParser url

        filter =
            case route of
                AlertsRoute filter_ ->
                    filter_

                SilenceListRoute filter_ ->
                    filter_

                _ ->
                    nullFilter

        prod =
            flags
                |> Json.decodeValue (Json.field "production" Json.bool)
                |> Result.withDefault False

        defaultCreator =
            flags
                |> Json.decodeValue (Json.field "defaultCreator" Json.string)
                |> Result.withDefault ""

        groupExpandAll =
            flags
                |> Json.decodeValue (Json.field "groupExpandAll" Json.bool)
                |> Result.withDefault False

        apiUrl =
            if prod then
                Api.makeApiUrl url.path

            else
                Api.makeApiUrl "http://localhost:9093/"

        libUrl =
            if prod then
                url.path

            else
                "/"
    in
    update (urlUpdate url)
        (Model
            (initSilenceList key (Time.millisToPosix 0))
            (initSilenceView key)
            (initSilenceForm key)
            (initAlertList key groupExpandAll)
            route
            filter
            initStatusModel
            url.path
            apiUrl
            libUrl
            Loading
            Loading
            defaultCreator
            groupExpandAll
            (Time.millisToPosix 0)
            key
        )


urlUpdate : Url -> Msg
urlUpdate url =
    let
        route =
            Parsing.urlParser url
    in
    case route of
        SilenceListRoute maybeFilter ->
            NavigateToSilenceList maybeFilter

        SilenceViewRoute silenceId ->
            NavigateToSilenceView silenceId

        SilenceFormEditRoute silenceId ->
            NavigateToSilenceFormEdit silenceId

        SilenceFormNewRoute params ->
            NavigateToSilenceFormNew params

        AlertsRoute filter ->
            NavigateToAlerts filter

        StatusRoute ->
            NavigateToStatus

        TopLevelRoute ->
            RedirectAlerts

        NotFoundRoute ->
            NavigateToNotFound


subscriptions : Model -> Sub Msg
subscriptions model =
    Time.every 1000 SetTime
