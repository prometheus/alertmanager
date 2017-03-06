module Main exposing (..)

import Navigation
import Task
import Time
import Parsing
import Views
import Alerts.Update
import Alerts.Types exposing (Route(Receiver))
import Types exposing (..)
import Utils.Types exposing (..)
import Silences.Api
import Silences.Types exposing (Silence, nullTime, nullSilence)
import Silences.Update
import Translators exposing (alertTranslator, silenceTranslator)


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
                    { text = Nothing, receiver = Nothing, showSilenced = Nothing }
    in
        update (urlUpdate location) (Model Loading Loading Loading route filter)


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        CreateSilenceFromAlert alert ->
            let
                silence =
                    { nullSilence | matchers = (List.map (\( k, v ) -> Matcher k v False) alert.labels) }
            in
                ( { model | silence = Success silence }, Cmd.none )

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
                ( { model | route = SilencesRoute silencesRoute, filter = filter }, Cmd.map silenceTranslator silencesCmd )

        Silences silencesMsg ->
            let
                ( silences, silence, silencesCmd ) =
                    Silences.Update.update silencesMsg model.silences model.silence model.filter
            in
                ( { model | silences = silences, silence = silence }, Cmd.map silenceTranslator silencesCmd )

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

            _ ->
                -- TODO: 404 page
                RedirectAlerts



-- SUBSCRIPTIONS
-- TODO: Poll API for changes.


subscriptions : Model -> Sub Msg
subscriptions model =
    Sub.none
