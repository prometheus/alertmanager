-- External Imports
import Navigation

-- Internal Imports
import Parsing
import Views
import Api
import Types exposing (..)

main =
  Navigation.program Parsing.urlParser
    { init = init
    , view = Views.view
    , update = update
    , urlUpdate = urlUpdate
    , subscriptions = subscriptions
    }


init : Route -> (Model, Cmd Msg)
init route =
  -- TODO: Correct empty state.
  urlUpdate route (Model
    [] -- Silences
    (Silence 0 "" "" "" "" "" [])
    [] -- Alerts
    (Alert "")
    route)
-- Update


update : Msg -> Model -> (Model, Cmd Msg)
update msg model =
  case msg of
    SilencesFetchSucceed silences ->
      ({ model | silences = silences }, Cmd.none)

    SilenceFetchSucceed silence ->
      ({ model | silence = silence }, Cmd.none)

    AlertsFetchSucceed alerts ->
      ({ model | alerts = alerts }, Cmd.none)

    AlertFetchSucceed alert ->
      ({ model | alert = alert }, Cmd.none)

    FetchFail fail ->
      let
        one = Debug.log "fail" fail
      in
        ({model | route = NotFound }, Cmd.none)


urlUpdate : Route -> Model -> (Model, Cmd Msg)
urlUpdate route model =
  let
      cmd =
        case route of
          SilencesRoute ->
            Api.getSilences

          TopLevel ->
            Navigation.modifyUrl "/#/alerts"

          _ ->
            Cmd.none
  in
    ({model | route = route }, cmd)


-- SUBSCRIPTIONS


-- TODO: Poll API for changes.
subscriptions : Model -> Sub Msg
subscriptions model =
  Sub.none


