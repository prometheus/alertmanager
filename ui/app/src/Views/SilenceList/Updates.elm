module Views.SilenceList.Updates exposing (update, urlUpdate)

import Navigation
import Silences.Api as Api
import Utils.Filter exposing (Filter, generateQueryString)
import Utils.Types as Types exposing (ApiData(Failure, Loading, Success), Matchers, Time)
import Views.FilterBar.Updates as FilterBar
import Views.SilenceList.Types exposing (Model, SilenceListMsg(..))


update : SilenceListMsg -> Model -> Filter -> String -> String -> ( Model, Cmd SilenceListMsg )
update msg model filter basePath apiUrl =
    case msg of
        SilencesFetch sils ->
            ( { model | silences = sils }, Cmd.none )

        FetchSilences ->
            ( { model
                | filterBar = FilterBar.setMatchers filter model.filterBar
                , silences = Loading
                , showConfirmationDialog = False
              }
            , Api.getSilences apiUrl filter SilencesFetch
            )

        ConfirmDestroySilence silence refresh ->
            ( { model | showConfirmationDialog = True }
            , Cmd.none
            )

        DestroySilence silence refresh ->
            -- TODO: "Deleted id: ID" growl
            -- TODO: Check why POST isn't there but is accepted
            { model | silences = Loading, showConfirmationDialog = False }
                ! [ Api.destroy apiUrl silence (always FetchSilences)
                  , if refresh then
                        Navigation.newUrl (basePath ++ "#/silences")
                    else
                        Cmd.none
                  ]

        MsgForFilterBar msg ->
            let
                ( filterBar, cmd ) =
                    FilterBar.update (basePath ++ "#/silences") filter msg model.filterBar
            in
                ( { model | filterBar = filterBar }, Cmd.map MsgForFilterBar cmd )

        SetTab tab ->
            ( { model | tab = tab }, Cmd.none )


urlUpdate : Maybe String -> ( SilenceListMsg, Filter )
urlUpdate maybeString =
    ( FetchSilences, updateFilter maybeString )


updateFilter : Maybe String -> Filter
updateFilter maybeFilter =
    { receiver = Nothing
    , showSilenced = Nothing
    , showInhibited = Nothing
    , group = Nothing
    , text = maybeFilter
    }
