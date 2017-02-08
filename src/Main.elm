module Main exposing (..)

-- External Imports

import Navigation
import Task
import Time
import ISO8601


-- Internal Imports

import Parsing
import Views
import Alerts.Update
import Types exposing (..)
import Utils.Types exposing (..)
import Utils.List
import Utils.Date
import Silences.Api
import Translators exposing (alertTranslator)


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
    in
        update (urlUpdate location) (Model Loading Loading Loading route "")


nullSilence : Silence
nullSilence =
    Silence 0 "" "" nullTime nullTime nullTime [ nullMatcher ]


nullMatcher : Matcher
nullMatcher =
    Matcher "" "" False


nullTime : Types.Time
nullTime =
    let
        epochString =
            ISO8601.toString Utils.Date.unixEpochStart
    in
        Types.Time Utils.Date.unixEpochStart epochString True


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        -- HTTP Result messages
        SilencesFetch silences ->
            ( { model | silences = silences }, Cmd.none )

        SilenceFetch silence ->
            ( { model | silence = silence }, Cmd.none )

        SilenceCreate (Ok id) ->
            ( { model | silence = Success nullSilence, route = SilenceRoute id }, Navigation.newUrl ("/#/silences/" ++ toString id) )

        SilenceCreate (Err err) ->
            ( { model | silence = Success nullSilence, route = SilencesRoute }, Navigation.newUrl "/#/silences" )

        SilenceDestroy (Ok id) ->
            -- TODO: "Deleted id: ID" growl
            -- TODO: Add DELETE to accepted CORS methods in alertmanager
            -- TODO: Check why POST isn't there but is accepted
            ( { model | route = SilencesRoute }, Navigation.newUrl "/#/silences" )

        SilenceDestroy (Err err) ->
            -- TODO: Add error to the message or something.
            ( { model | route = SilencesRoute, error = "Failed to destroy silence" }, Navigation.newUrl "/#/silences" )

        CreateSilenceFromAlert alert ->
            let
                silence =
                    { nullSilence | matchers = (List.map (\( k, v ) -> Matcher k v False) alert.labels) }
            in
                ( { model | silence = Success silence }, Cmd.none )

        NavigateToAlerts alertsRoute ->
            let
                alertsMsg =
                    (Alerts.Update.urlUpdate alertsRoute)

                ( alertGroups, alertCmd ) =
                    Alerts.Update.update alertsMsg model.alertGroups
            in
                ( { model | alertGroups = alertGroups, route = AlertsRoute alertsRoute }, Cmd.map alertTranslator alertCmd )

        Alerts alertsMsg ->
            let
                ( alertGroups, alertCmd ) =
                    Alerts.Update.update alertsMsg model.alertGroups
            in
                ( { model | alertGroups = alertGroups }, Cmd.map alertTranslator alertCmd )

        -- API interaction messages
        FetchSilences ->
            ( { model | silences = Loading, route = SilencesRoute }, Silences.Api.getSilences )

        FetchSilence id ->
            ( { model | silence = Loading, route = SilenceRoute id }, Silences.Api.getSilence id )

        EditSilence id ->
            -- Look into setting the silence if we're moving from the list to
            -- edit view, so that there's no pause for users navigating around.
            ( { model | route = EditSilenceRoute id }, Silences.Api.getSilence id )

        CreateSilence silence ->
            ( model, Silences.Api.create silence )

        DestroySilence silence ->
            ( model, Silences.Api.destroy silence )

        NewSilence ->
            ( { model | route = NewSilenceRoute }, (Task.perform (NewDefaultTimeRange nullSilence) Time.now) )

        RedirectAlerts ->
            ( model, Navigation.newUrl "/#/alerts" )

        -- New silence form messages
        UpdateStartsAt silence time ->
            -- TODO:
            -- Update silence to hold datetime as string, on each pass through
            -- here update an error message "this is invalid", but let them put
            -- it in anyway.
            let
                startsAt =
                    Utils.Date.toISO8601Time time
            in
                ( { model | silence = Success { silence | startsAt = startsAt } }, Cmd.none )

        UpdateEndsAt silence time ->
            let
                endsAt =
                    Utils.Date.toISO8601Time time
            in
                ( { model | silence = (Success { silence | endsAt = endsAt }) }, Cmd.none )

        UpdateCreatedBy silence by ->
            ( { model | silence = Success { silence | createdBy = by } }, Cmd.none )

        UpdateComment silence comment ->
            ( { model | silence = Success { silence | comment = comment } }, Cmd.none )

        AddMatcher silence ->
            -- TODO: If a user adds two blank matchers and attempts to update
            -- one, both are updated because they are identical. Maybe add a
            -- unique identifier on creation so this doesn't happen.
            ( { model | silence = Success { silence | matchers = silence.matchers ++ [ Matcher "" "" False ] } }, Cmd.none )

        DeleteMatcher silence matcher ->
            let
                -- TODO: This removes all empty matchers. Maybe just remove the
                -- one that was clicked.
                newSil =
                    { silence | matchers = (List.filter (\x -> x /= matcher) silence.matchers) }
            in
                ( { model | silence = Success newSil }, Cmd.none )

        UpdateMatcherName silence matcher name ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | name = name } silence.matchers
            in
                ( { model | silence = Success { silence | matchers = matchers } }, Cmd.none )

        UpdateMatcherValue silence matcher value ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | value = value } silence.matchers
            in
                ( { model | silence = Success { silence | matchers = matchers } }, Cmd.none )

        UpdateMatcherRegex silence matcher bool ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | isRegex = bool } silence.matchers
            in
                ( { model | silence = Success { silence | matchers = matchers } }, Cmd.none )

        NewDefaultTimeRange silence time ->
            let
                endsIso =
                    Utils.Date.addTime time (2 * Time.hour)

                endsAt =
                    Types.Time endsIso (ISO8601.toString endsIso) True

                startsIso =
                    Utils.Date.toISO8601 time

                startsAt =
                    Types.Time endsIso (ISO8601.toString startsIso) True

                s =
                    model.silence
            in
                ( { model | silence = Success { silence | startsAt = startsAt, endsAt = endsAt } }, Cmd.none )

        Noop ->
            ( model, Cmd.none )


urlUpdate : Navigation.Location -> Msg
urlUpdate location =
    let
        route =
            Parsing.urlParser location
    in
        case route of
            SilencesRoute ->
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
