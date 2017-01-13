module Main exposing (..)

-- External Imports

import Date
import Navigation
import Task
import Time
import ISO8601


-- Internal Imports

import Parsing
import Views
import Api
import Types exposing (..)
import Utils.List
import Utils.Date
import Silences.Api


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
        update (urlUpdate location) (Model [] nullSilence [] route "")


nullSilence : Silence
nullSilence =
    Silence 0 "" "" Utils.Date.unixEpochStart Utils.Date.unixEpochStart Utils.Date.unixEpochStart [ nullMatcher ]


nullMatcher : Matcher
nullMatcher =
    Matcher "" "" False


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        SilencesFetch (Ok silences) ->
            ( { model | silences = silences }, Cmd.none )

        SilencesFetch (Err _) ->
            ( { model | route = NotFound }, Cmd.none )

        SilenceFetch (Ok silence) ->
            ( { model | silence = silence }, Cmd.none )

        SilenceFetch (Err _) ->
            -- show the error somehow
            ( { model | route = NotFound }, Cmd.none )

        SilenceCreate (Ok id) ->
            ( { model | silence = nullSilence, route = SilenceRoute id }, Navigation.newUrl ("/#/silences/" ++ toString id) )

        SilenceCreate (Err err) ->
            ( { model | silence = nullSilence, route = SilencesRoute }, Navigation.newUrl "/#/silences" )

        SilenceDestroy (Ok id) ->
            -- TODO: "Deleted id: ID" growl
            -- TODO: Add DELETE to accepted CORS methods in alertmanager
            -- TODO: Check why POST isn't there but is accepted
            ( { model | route = SilencesRoute }, Navigation.newUrl "/#/silences" )

        SilenceDestroy (Err err) ->
            -- TODO: Add error to the message or something.
            ( { model | route = SilencesRoute, error = "Failed to destroy silence" }, Navigation.newUrl "/#/silences" )

        FetchSilences ->
            ( { model | silence = nullSilence, route = SilencesRoute }, Silences.Api.getSilences )

        FetchSilence id ->
            ( { model | route = SilenceRoute id }, Silences.Api.getSilence id )

        EditSilence id ->
            ( { model | route = EditSilenceRoute id }, Silences.Api.getSilence id )

        NewSilence ->
            ( { model | route = NewSilenceRoute }, (Task.perform NewDefaultTimeRange Time.now) )

        SilenceFromAlert matchers ->
            let
                s =
                    { nullSilence | matchers = List.sortBy .name matchers }
            in
                ( { model | silence = s }, (Task.perform NewDefaultTimeRange Time.now) )

        CreateSilence silence ->
            ( model, Api.createSilence silence )

        DestroySilence silence ->
            ( model, Api.destroySilence silence )

        FetchAlertGroups ->
            ( { model | silence = nullSilence, route = AlertGroupsRoute }, Api.getAlertGroups )

        AlertGroupsFetch (Ok alertGroups) ->
            ( { model | alertGroups = alertGroups }, Cmd.none )

        AlertGroupsFetch (Err err) ->
            ( { model | route = NotFound }, Cmd.none )

        RedirectAlerts ->
            ( { model | route = AlertGroupsRoute }, Navigation.newUrl "/#/alerts" )

        UpdateStartsAt time ->
            let
                sil =
                    model.silence

                startsAt =
                    Utils.Date.parseWithDefault sil.startsAt time
            in
                ( { model | silence = { sil | startsAt = startsAt } }, Cmd.none )

        UpdateEndsAt time ->
            let
                sil =
                    model.silence

                endsAt =
                    Utils.Date.parseWithDefault sil.endsAt time
            in
                ( { model | silence = { sil | endsAt = endsAt } }, Cmd.none )

        UpdateCreatedBy by ->
            let
                sil =
                    model.silence
            in
                ( { model | silence = { sil | createdBy = by } }, Cmd.none )

        UpdateComment comment ->
            let
                sil =
                    model.silence
            in
                ( { model | silence = { sil | comment = comment } }, Cmd.none )

        AddMatcher ->
            -- TODO: If a user adds two blank matchers and attempts to update
            -- one, both are updated because they are identical. Maybe add a
            -- unique identifier on creation so this doesn't happen.
            let
                sil =
                    model.silence
            in
                ( { model | silence = { sil | matchers = sil.matchers ++ [ Matcher "" "" False ] } }, Cmd.none )

        DeleteMatcher matcher ->
            let
                s =
                    model.silence

                -- TODO: This removes all empty matchers. Maybe just remove the
                -- one that was clicked.
                newSil =
                    { s | matchers = (List.filter (\x -> x /= matcher) s.matchers) }
            in
                ( { model | silence = newSil }, Cmd.none )

        UpdateMatcherName matcher name ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | name = name } model.silence.matchers

                s =
                    model.silence
            in
                ( { model | silence = { s | matchers = matchers } }, Cmd.none )

        UpdateMatcherValue matcher value ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | value = value } model.silence.matchers

                s =
                    model.silence
            in
                ( { model | silence = { s | matchers = matchers } }, Cmd.none )

        UpdateMatcherRegex matcher bool ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | isRegex = bool } model.silence.matchers

                s =
                    model.silence
            in
                ( { model | silence = { s | matchers = matchers } }, Cmd.none )

        NewDefaultTimeRange time ->
            let
                endsAt =
                    Utils.Date.addTime time (2 * Time.hour)

                startsAt =
                    Utils.Date.toISO8601 time

                s =
                    model.silence
            in
                ( { model | silence = { s | startsAt = startsAt, endsAt = endsAt } }, Cmd.none )

        Noop _ ->
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

            AlertGroupsRoute ->
                FetchAlertGroups

            _ ->
                -- TODO: 404 page
                RedirectAlerts



-- SUBSCRIPTIONS
-- TODO: Poll API for changes.


subscriptions : Model -> Sub Msg
subscriptions model =
    Sub.none
