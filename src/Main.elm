-- External Imports


module Main exposing (..)

import Navigation


-- Internal Imports

import Parsing
import Views
import Api
import Types exposing (..)


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
        ( Model [] (Silence 0 "" "" "" "" "" []) [] (Alert "") route, routeCmd route )


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

        RedirectAlerts ->
            ( { model | route = AlertsRoute }, Navigation.newUrl "/#/alerts" )


routeCmd : Route -> Cmd Msg
routeCmd route =
    case route of
        SilencesRoute ->
            Api.getSilences

        SilenceRoute id ->
            Api.getSilence id

        _ ->
            Cmd.none


urlUpdate : Navigation.Location -> Msg
urlUpdate location =
    let
        route =
            Parsing.urlParser location
    in
        case route of
            SilencesRoute ->
                FetchSilences

            SilenceRoute id ->
                FetchSilence id

            _ ->
                -- TODO: 404 page
                RedirectAlerts



-- SUBSCRIPTIONS
-- TODO: Poll API for changes.


subscriptions : Model -> Sub Msg
subscriptions model =
    Sub.none
