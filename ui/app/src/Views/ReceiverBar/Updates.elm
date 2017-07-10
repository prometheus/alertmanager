module Views.ReceiverBar.Updates exposing (update, fetchReceivers)

import Views.ReceiverBar.Types exposing (Model, Msg(..))
import Utils.Types exposing (ApiData(Success))
import Utils.Filter exposing (Filter, generateQueryString, stringifyGroup, parseGroup)
import Navigation
import Regex
import Alerts.Api as Api


update : String -> Filter -> Msg -> Model -> ( Model, Cmd Msg )
update url filter msg model =
    case msg of
        ReceiversFetched (Success receivers) ->
            ( { model | receivers = receivers }, Cmd.none )

        ReceiversFetched _ ->
            ( model, Cmd.none )

        ToggleReceivers show ->
            ( { model | showRecievers = show }, Cmd.none )

        SelectReceiver receiver ->
            ( { model | showRecievers = False }
            , Navigation.newUrl (url ++ generateQueryString { filter | receiver = Maybe.map Regex.escape receiver })
            )


fetchReceivers : String -> Cmd Msg
fetchReceivers =
    Api.fetchReceivers >> Cmd.map ReceiversFetched
