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
        update (urlUpdate location) (Model [] nullSilence [] route)


nullSilence : Silence
nullSilence =
    Silence 0 "" "" "" "" "" []


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

        RedirectSilences ->
            ( { model | route = AlertGroupsRoute }, Navigation.newUrl "/#/silences" )


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
                RedirectSilences



-- SUBSCRIPTIONS
-- TODO: Poll API for changes.


subscriptions : Model -> Sub Msg
subscriptions model =
    Sub.none
