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
import Utils.Parsing
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

                -- TODO: Extract silences routes to silences namespace
                SilencesRoute maybeQuery ->
                    { receiver = Nothing
                    , showSilenced = Nothing
                    , text = maybeQuery
                    , matchers = Utils.Parsing.parseLabels maybeQuery
                    }

                _ ->
                    { text = Nothing, matchers = Nothing, receiver = Nothing, showSilenced = Nothing }
    in
        update (urlUpdate location) (Model Loading Loading Loading route filter)


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        SilencesFetch silences ->
            ( { model | silences = silences }, Cmd.none )

        SilenceFetch silence ->
            ( { model | silence = silence }, Cmd.none )

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

                ( alertGroups, pf, alertCmd ) =
                    Alerts.Update.update alertsMsg model.alertGroups filter
            in
                ( { model | alertGroups = alertGroups, filter = pf, route = AlertsRoute alertsRoute }, Cmd.map alertTranslator alertCmd )

        Alerts alertsMsg ->
            let
                ( alertGroups, filter, alertCmd ) =
                    Alerts.Update.update alertsMsg model.alertGroups model.filter
            in
                ( { model | alertGroups = alertGroups, filter = filter }, Cmd.map alertTranslator alertCmd )

        Silences silencesMsg ->
            let
                ( silences, silence, silenceCmd ) =
                    Silences.Update.update silencesMsg model.silences model.silence model.filter
            in
                ( { model | silences = silences, silence = silence }, Cmd.map silenceTranslator silenceCmd )

        FetchSilences ->
            ( { model | silences = model.silences, route = (SilencesRoute model.filter.text) }, Silences.Api.getSilences )

        FetchSilence id ->
            ( { model | silence = Loading, route = SilenceRoute id }, Silences.Api.getSilence id )

        EditSilence id ->
            -- Look into setting the silence if we're moving from the list to
            -- edit view, so that there's no pause for users navigating around.
            ( { model | silence = Loading, route = EditSilenceRoute id }, Silences.Api.getSilence id )

        NewSilence ->
            ( { model | route = NewSilenceRoute }, (Task.perform Silences.Types.NewDefaultTimeRange Time.now) |> Cmd.map Silences )

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

        ParseFilterText ->
            let
                filter =
                    model.filter

                f =
                    { filter | matchers = Utils.Parsing.parseLabels filter.text }
            in
                ( { model | filter = f }, Cmd.none )

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
            SilencesRoute _ ->
                FetchSilences

            NewSilenceRoute ->
                NewSilence

            SilenceRoute id ->
                FetchSilence id

            EditSilenceRoute id ->
                EditSilence id

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
