module Views.SilenceList.Views exposing (filterSilencesByState, groupSilencesByState, silencesView, states, tabView, tabsView, view)

import Data.GettableSilence exposing (GettableSilence)
import Data.SilenceStatus exposing (State(..))
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Keyed
import Html.Lazy exposing (lazy, lazy2, lazy3)
import Silences.Types exposing (stateToString)
import Types exposing (Msg(..))
import Utils.String as StringUtils
import Utils.Types exposing (ApiData(..), Matcher)
import Utils.Views exposing (buttonLink, checkbox, error, formField, formInput, iconButtonMsg, loading, textField)
import Views.FilterBar.Views as FilterBar
import Views.SilenceList.SilenceView
import Views.SilenceList.Types exposing (Model, SilenceListMsg(..), SilenceTab)


view : Model -> Html Msg
view { filterBar, tab, silences, showConfirmationDialog } =
    div []
        [ div [ class "mb-4" ]
            [ label [ class "mb-2", for "filter-bar-matcher" ] [ text "Filter" ]
            , Html.map (MsgForFilterBar >> MsgForSilenceList) (FilterBar.view filterBar)
            ]
        , lazy2 tabsView tab silences
        , lazy3 silencesView showConfirmationDialog tab silences
        ]


tabsView : State -> ApiData (List SilenceTab) -> Html Msg
tabsView currentTab tabs =
    case tabs of
        Success silencesTabs ->
            List.map (\{ tab, count } -> tabView currentTab count tab) silencesTabs
                |> ul [ class "nav nav-tabs mb-4" ]

        _ ->
            List.map (tabView currentTab 0) states
                |> ul [ class "nav nav-tabs mb-4" ]


tabView : State -> Int -> State -> Html Msg
tabView currentTab count tab =
    Utils.Views.tab tab currentTab (SetTab >> MsgForSilenceList) <|
        case count of
            0 ->
                [ text (StringUtils.capitalizeFirst (stateToString tab)) ]

            n ->
                [ text (StringUtils.capitalizeFirst (stateToString tab))
                , span
                    [ class "badge badge-pillow badge-default align-text-top ml-2" ]
                    [ text (String.fromInt n) ]
                ]


silencesView : Maybe String -> State -> ApiData (List SilenceTab) -> Html Msg
silencesView showConfirmationDialog tab silencesTab =
    case silencesTab of
        Success tabs ->
            tabs
                |> List.filter (.tab >> (==) tab)
                |> List.head
                |> Maybe.map .silences
                |> Maybe.withDefault []
                |> (\silences ->
                        if List.isEmpty silences then
                            Utils.Views.error "No silences found"

                        else
                            Html.Keyed.ul [ class "list-group" ]
                                (List.map
                                    (\silence ->
                                        ( silence.id
                                        , Views.SilenceList.SilenceView.view
                                            (showConfirmationDialog == Just silence.id)
                                            silence
                                        )
                                    )
                                    silences
                                )
                   )

        Failure msg ->
            error msg

        _ ->
            loading


groupSilencesByState : List GettableSilence -> List ( State, List GettableSilence )
groupSilencesByState silences =
    List.map (\state -> ( state, filterSilencesByState state silences )) states


states : List State
states =
    [ Active, Pending, Expired ]


filterSilencesByState : State -> List GettableSilence -> List GettableSilence
filterSilencesByState state =
    List.filter (filterSilenceByState state)


filterSilenceByState : State -> GettableSilence -> Bool
filterSilenceByState state silence =
    silence.status.state == state
