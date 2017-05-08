module Views.SilenceList.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick, onInput)
import Views.SilenceList.Types exposing (SilenceListMsg(..), Model)
import Views.Shared.SilenceBase
import Silences.Types exposing (Silence)
import Utils.Types exposing (Matcher, ApiResponse(..), ApiData)
import Utils.Filter exposing (Filter)
import Utils.Views exposing (iconButtonMsg, checkbox, textField, formInput, formField, buttonLink, error, loading)
import Time
import Types exposing (Msg(UpdateFilter, MsgForSilenceList, Noop))
import Views.FilterBar.Views as FilterBar
import Views.FilterBar.Types as FilterBarTypes


view : Model -> Time.Time -> Html Msg
view model currentTime =
    case model.silences of
        Success sils ->
            -- Add buttons at the top to filter Active/Pending/Expired
            silences sils model.filterBar (text "")

        Loading ->
            loading

        Failure msg ->
            silences [] model.filterBar (error msg)


silences : List Silence -> FilterBarTypes.Model -> Html Msg -> Html Msg
silences silences filterBar errorHtml =
    let
        html =
            if List.isEmpty silences then
                div [ class "mt2" ] [ text "no silences found" ]
            else
                ul
                    [ classList
                        [ ( "list", True )
                        , ( "pa0", True )
                        ]
                    ]
                    (List.map silenceList silences)
    in
        div []
            [ Html.map (MsgForFilterBar >> MsgForSilenceList) (FilterBar.view filterBar)
            , a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", href "#/silences/new" ] [ text "New Silence" ]
            , errorHtml
            , html
            ]


silenceList : Silence -> Html Msg
silenceList silence =
    li
        [ class "pa3 pa4-ns bb b--black-10" ]
        [ Views.Shared.SilenceBase.view silence
        ]
