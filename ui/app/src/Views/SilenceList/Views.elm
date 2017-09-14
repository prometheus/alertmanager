module Views.SilenceList.Views exposing (..)

import Html exposing (..)
import Html.Attributes exposing (..)
import Silences.Types exposing (Silence, State(..), stateToString)
import Types exposing (Msg(MsgForSilenceList, Noop, UpdateFilter))
import Utils.Api exposing (withDefault)
import Utils.String as StringUtils
import Utils.Types exposing (ApiData(..), Matcher)
import Utils.Views exposing (buttonLink, checkbox, error, formField, formInput, iconButtonMsg, loading, textField)
import Views.FilterBar.Views as FilterBar
import Views.SilenceList.SilenceView
import Views.SilenceList.Types exposing (Model, SilenceListMsg(..))


view : Model -> Html Msg
view { filterBar, tab, silences, showConfirmationDialog } =
    div []
        [ div [ class "mb-4" ]
            [ label [ class "mb-2", for "filter-bar-matcher" ] [ text "Filter" ]
            , Html.map (MsgForFilterBar >> MsgForSilenceList) (FilterBar.view filterBar)
            ]
        , ul [ class "nav nav-tabs mb-4" ]
            (List.map (tabView tab) (groupSilencesByState (withDefault [] silences)))
        , case silences of
            Success sils ->
                silencesView showConfirmationDialog (filterSilencesByState tab sils)

            Failure msg ->
                error msg

            _ ->
                loading
        ]


tabView : State -> ( State, List a ) -> Html Msg
tabView currentState ( state, silences ) =
    Utils.Views.tab state currentState (SetTab >> MsgForSilenceList) <|
        case List.length silences of
            0 ->
                [ text (StringUtils.capitalizeFirst (stateToString state)) ]

            n ->
                [ text (StringUtils.capitalizeFirst (stateToString state))
                , span
                    [ class "badge badge-pillow badge-default align-text-top ml-2" ]
                    [ text (toString n) ]
                ]


silencesView : Bool -> List Silence -> Html Msg
silencesView showConfirmationDialog silences =
    if List.isEmpty silences then
        div [] [ text "No silences found" ]
    else
        ul [ class "list-group" ]
            (List.map (Views.SilenceList.SilenceView.view showConfirmationDialog) silences)


groupSilencesByState : List Silence -> List ( State, List Silence )
groupSilencesByState silences =
    List.map (\state -> ( state, filterSilencesByState state silences )) states


states : List State
states =
    [ Active, Pending, Expired ]


filterSilencesByState : State -> List Silence -> List Silence
filterSilencesByState state =
    List.filter (.status >> .state >> (==) state)
