-- External Imports


module Main exposing (..)

import Navigation


-- Internal Imports

import Parsing
import Views
import Api
import Types exposing (..)
import Utils.List


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
        update (urlUpdate location) (Model [] nullSilence [] route)


nullSilence : Silence
nullSilence =
    Silence 0 "" "" "" "" "" [ nullMatcher ]


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

        FetchSilences ->
            ( { model | route = SilencesRoute }, Api.getSilences )

        FetchSilence id ->
            ( { model | route = SilenceRoute id }, Api.getSilence id )

        EditSilence id ->
            ( { model | route = EditSilenceRoute id }, Api.getSilence id )

        NewSilence ->
            ( { model | silence = nullSilence, route = NewSilenceRoute }, Cmd.none )

        FetchAlertGroups ->
            ( { model | route = AlertGroupsRoute }, Api.getAlertGroups )

        AlertGroupsFetch (Ok alertGroups) ->
            ( { model | alertGroups = alertGroups }, Cmd.none )

        AlertGroupsFetch (Err err) ->
            let
                one =
                    Debug.log "error" err
            in
                ( { model | route = NotFound }, Cmd.none )

        RedirectAlerts ->
            ( { model | route = AlertGroupsRoute }, Navigation.newUrl "/#/alerts" )

        AddMatcher ->
            -- TODO: If a user adds two blank matchers and attempts to update
            -- one, both are updated because they are identical. Maybe add a
            -- unique identifier on creation so this doesn't happen.
            let
                sil =
                    model.silence

                newSil =
                    { sil | matchers = sil.matchers ++ [ Matcher "" "" False ] }
            in
                ( { model | silence = newSil }, Cmd.none )

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

                newSil =
                    { s | matchers = matchers }
            in
                ( { model | silence = newSil }, Cmd.none )

        UpdateMatcherValue matcher value ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | value = value } model.silence.matchers

                s =
                    model.silence

                newSil =
                    { s | matchers = matchers }
            in
                ( { model | silence = newSil }, Cmd.none )

        UpdateMatcherRegex matcher bool ->
            let
                matchers =
                    Utils.List.replaceIf (\x -> x == matcher) { matcher | isRegex = bool } model.silence.matchers

                s =
                    model.silence

                newSil =
                    { s | matchers = matchers }
            in
                ( { model | silence = newSil }, Cmd.none )


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
