module Main exposing (..)

import Navigation
import Task
import Time
import Parsing
import Views
import Alerts.Update
import Alerts.Api
import Types exposing (..)
import Utils.Types exposing (..)
import Utils.List
import Silences.Types exposing (Silence, nullTime, nullSilence)
import Silences.Update
import Translators exposing (alertTranslator, silenceTranslator)
import Status.Types exposing (StatusModel)
import Status.Update exposing (update)
import Status.Api exposing (getStatus)


main : Program Never Model Msg
main =
    Navigation.program urlUpdate
        { init = init
        , update = update
        , view = Views.view
        , subscriptions = subscriptions
        }


init : Navigation.Location -> ( Model, Cmd Msg )
init location =
    let
        route =
            Parsing.urlParser location

        filter =
            case route of
                AlertsRoute alertsRoute ->
                    Alerts.Update.updateFilter alertsRoute

                SilencesRoute silencesRoute ->
                    Silences.Update.updateFilter silencesRoute

                _ ->
                    nullFilter

        ( model, msg ) =
            update (urlUpdate location) (Model Loading Loading Loading route filter 0 (StatusModel Nothing))
    in
        model ! [ msg, Task.perform UpdateCurrentTime Time.now ]


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        CreateSilenceFromAlert alert ->
            let
                silence =
                    { nullSilence | matchers = (List.map (\( k, v ) -> Matcher k v False) alert.labels) }
            in
                ( { model | silence = Success silence }, Cmd.none )

        PreviewSilence silence ->
            let
                s =
                    { silence | silencedAlertGroups = Loading }

                filter =
                    { nullFilter | text = Just <| Utils.List.mjoin s.matchers }
            in
                ( { model | silence = Success silence }, Cmd.map alertTranslator (Alerts.Api.alertPreview filter) )

        AlertGroupsPreview alertGroups ->
            let
                silence =
                    case model.silence of
                        Success sil ->
                            Success { sil | silencedAlertGroups = alertGroups }

                        Failure e ->
                            Failure e

                        Loading ->
                            Loading
            in
                ( { model | silence = silence }, Cmd.none )

        NavigateToAlerts alertsRoute ->
            let
                ( alertsMsg, filter ) =
                    (Alerts.Update.urlUpdate alertsRoute)

                ( alertGroups, alertCmd ) =
                    Alerts.Update.update alertsMsg model.alertGroups filter
            in
                ( { model | alertGroups = alertGroups, route = AlertsRoute alertsRoute, filter = filter }, Cmd.map alertTranslator alertCmd )

        Alerts alertsMsg ->
            let
                ( alertGroups, alertCmd ) =
                    Alerts.Update.update alertsMsg model.alertGroups model.filter
            in
                ( { model | alertGroups = alertGroups }, Cmd.map alertTranslator alertCmd )

        NavigateToSilences silencesRoute ->
            let
                ( silencesMsg, filter ) =
                    (Silences.Update.urlUpdate silencesRoute)

                ( silences, silence, silencesCmd ) =
                    Silences.Update.update silencesMsg model.silences model.silence filter
            in
                ( { model | silence = silence, silences = silences, route = SilencesRoute silencesRoute, filter = filter }
                , Cmd.map silenceTranslator silencesCmd
                )

        NavigateToStatus ->
            ( { model | route = StatusRoute }, getStatus )

        Silences silencesMsg ->
            let
                ( silences, silence, silencesCmd ) =
                    Silences.Update.update silencesMsg model.silences model.silence model.filter
            in
                ( { model | silences = silences, silence = silence }
                , Cmd.map silenceTranslator silencesCmd
                )

        RedirectAlerts ->
            ( model, Task.perform NewUrl (Task.succeed "/#/alerts") )

        UpdateFilter filter text ->
            let
                t =
                    if text == "" then
                        Nothing
                    else
                        Just text
            in
                ( { model | filter = { filter | text = t } }, Cmd.none )

        NewUrl url ->
            ( model, Navigation.newUrl url )

        Noop ->
            ( model, Cmd.none )

        UpdateCurrentTime time ->
            ( { model | currentTime = time }, Cmd.none )

        MsgForStatus msg ->
            let
                ( status, cmd ) =
                    Status.Update.update msg model.status
            in
                ( { model | status = status }, cmd )


urlUpdate : Navigation.Location -> Msg
urlUpdate location =
    let
        route =
            Parsing.urlParser location
    in
        case route of
            SilencesRoute silencesRoute ->
                NavigateToSilences silencesRoute

            AlertsRoute alertsRoute ->
                NavigateToAlerts alertsRoute

            StatusRoute ->
                NavigateToStatus

            _ ->
                -- TODO: 404 page
                RedirectAlerts



-- SUBSCRIPTIONS
-- TODO: Poll API for changes.


subscriptions : Model -> Sub Msg
subscriptions model =
    Sub.none
